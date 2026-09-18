package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
	"github.com/ADHFMZ7/crypto-exchange/internal/stream"
	"github.com/jackc/pgx/v5/pgconn"
)

type TradeService struct {
	WalletStore *stores.WalletStore
	UserStore   *stores.UserStore
	TradeStore  *stores.TradeStore
	OutboxStore *stores.OutboxStore

	MarketRegistry *market.Registry

	// Where settled trades are announced. Optional: nil simply means nobody is
	// listening, which is the normal state in tests and for a process with no
	// stream endpoint mounted.
	Stream *stream.Hub

	SettlementChan chan models.LedgerEvent
}

func NewTradeService(userStore *stores.UserStore, walletStore *stores.WalletStore, tradeStore *stores.TradeStore, outboxStore *stores.OutboxStore, registry *market.Registry, SChan chan models.LedgerEvent, hub *stream.Hub) *TradeService {

	service := &TradeService{
		WalletStore: walletStore,
		UserStore:   userStore,
		TradeStore:  tradeStore,
		OutboxStore: outboxStore,

		MarketRegistry: registry,
		SettlementChan: SChan,
		Stream:         hub,
	}

	go service.SettlementWorker()

	return service
}

func (service *TradeService) SettlementWorker() {

	// TODO: Figure out where this should come from?
	ctx := context.Background()

	log.Println("settlement: worker started")

	for event := range service.SettlementChan {
		service.absorb(ctx, event)
	}

	log.Println("settlement: worker stopped, no further events will be applied")
}

// How hard to try before writing an event off. The delays are short because the
// failure this exists for is a moment's database trouble, not an outage — an
// outage is what the pending rows and the next boot's replay are for.
const (
	settlementAttempts = 5
	settlementBackoff  = 60 * time.Millisecond
)

// absorb records one effect and then applies it.
//
// Recording first is the whole point. Until the row exists, a failure loses the
// engine's decision with nothing left to show for it; once it exists, a failure
// is a delay, and the worst case is a row somebody has to look at.
func (service *TradeService) absorb(ctx context.Context, event models.LedgerEvent) {
	id, err := service.OutboxStore.Append(ctx, event)
	if err != nil {
		// The database will not even take a note. Applying is hopeless, and
		// there is nowhere durable left to put this — the next boot's flush is
		// what makes the two sides agree again.
		log.Printf("settlement: could not record %s, dropping it: %v", describe(event), err)
		return
	}

	service.applyRecorded(ctx, id, event)
}

// applyRecorded works one recorded event until it lands or is written off.
//
// It blocks the worker while it retries, which backs the channel up and can
// eventually stall matching. That is the intended trade: an engine that keeps
// matching into a ledger that has stopped recording is producing divergence as
// fast as it can.
func (service *TradeService) applyRecorded(ctx context.Context, id int64, event models.LedgerEvent) {
	for attempt := 1; ; attempt++ {
		err := service.apply(ctx, id, event)

		// The effect marks its own event applied, in the same transaction, so
		// there is no window here in which it has landed but still looks owed.
		// ErrAlreadyApplied means some earlier attempt got there first — the
		// ledger already reflects this event, which is all that was wanted.
		if err == nil || errors.Is(err, stores.ErrAlreadyApplied) {
			return
		}

		if permanent(err) || attempt >= settlementAttempts {
			log.Printf("settlement: GIVING UP on event %d after %d attempt(s) — %s: %v",
				id, attempt, describe(event), err)
			if err := service.OutboxStore.MarkFailed(ctx, id, err.Error()); err != nil {
				log.Printf("settlement: could not mark event %d failed: %v", id, err)
			}
			return
		}

		_ = service.OutboxStore.RecordAttempt(ctx, id, err.Error())
		time.Sleep(time.Duration(attempt) * settlementBackoff)
	}
}

// apply routes an event to the half of the ledger it belongs to.
func (service *TradeService) apply(ctx context.Context, eventID int64, event models.LedgerEvent) error {
	switch {
	case event.Fill != nil:
		return service.settleFill(ctx, eventID, *event.Fill)
	case event.Cancel != nil:
		return service.releaseCancelled(ctx, eventID, *event.Cancel)
	}
	return models.ErrEmptyLedgerEvent
}

// permanent reports whether retrying could ever help.
//
// Two families qualify. Ours: an unknown market, a side that is not a side, a
// notional that cannot be computed — the event is malformed and will be just as
// malformed in a second. And Postgres integrity violations (SQLSTATE class 23),
// which mean the data is wrong rather than the moment: a fill that would
// overshoot its order, or a lock that would go negative.
//
// Everything else — a dropped connection, a lock timeout, a restarting database
// — is worth another go. Guessing wrong in this direction costs a few
// milliseconds; guessing wrong the other way writes off a fill that would have
// applied on the next attempt.
func permanent(err error) bool {
	if errors.Is(err, ErrUnknownMarket) ||
		errors.Is(err, models.ErrEmptyLedgerEvent) ||
		errors.Is(err, stores.ErrNoBalance) ||
		errors.Is(err, market.ErrNotPositive) ||
		errors.Is(err, market.ErrOverflow) ||
		errors.Is(err, market.ErrInvalidSide) {
		return true
	}

	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.HasPrefix(pgErr.Code, "23")
}

// ReplayPending applies everything the ledger is still owed from a previous run.
//
// It must run BEFORE the startup flush. A pending fill advances an order and
// draws down its lock; if the flush had already cancelled that order and zeroed
// locked_remaining, applying the fill would drive it negative and be refused by
// orders_locked_remaining_non_negative. Owed first, then unwind what is left.
func (service *TradeService) ReplayPending(ctx context.Context) error {
	pending, err := service.OutboxStore.Pending(ctx, 10_000)
	if err != nil {
		return fmt.Errorf("reading pending ledger events: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	log.Printf("startup: %d ledger event(s) owed from a previous run", len(pending))

	for _, row := range pending {
		event, err := stores.DecodeEvent(row.Kind, row.Payload)
		if err != nil {
			log.Printf("startup: event %d is unreadable (%s): %v", row.ID, row.Kind, err)
			if err := service.OutboxStore.MarkFailed(ctx, row.ID, err.Error()); err != nil {
				return err
			}
			continue
		}
		service.applyRecorded(ctx, row.ID, event)
	}

	failed, err := service.OutboxStore.FailedCount(ctx)
	if err != nil {
		return err
	}
	if failed > 0 {
		log.Printf("startup: WARNING %d ledger event(s) were never applied — "+
			"the book that produced them is gone, so this is a divergence only a person can close",
			failed)
	}

	return nil
}

// describe names an event for a log line, since a dropped one leaves nothing else.
func describe(event models.LedgerEvent) string {
	switch {
	case event.Fill != nil:
		f := event.Fill
		return fmt.Sprintf("fill on %s (orders %d/%d, %d @ %d)",
			f.Market, f.RestingOrderID, f.IncomingOrderID, f.Quantity, f.Price)
	case event.Cancel != nil:
		return fmt.Sprintf("cancellation of order %d on %s", event.Cancel.OrderID, event.Cancel.Market)
	}
	return "an empty event"
}

// settleFill applies one execution to the ledger.
func (service *TradeService) settleFill(ctx context.Context, eventID int64, trade models.Trade) error {

	m, ok := service.MarketRegistry.BySymbol(trade.Market)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownMarket, trade.Market)
	}

	quoteAmount, err := m.FillNotional(trade.Quantity, trade.Price)
	if err != nil {
		return fmt.Errorf("notional: %w", err)
	}

	tradeID, err := service.TradeStore.Settle(ctx, eventID, trade, m.Base.Code, m.Quote.Code, quoteAmount)
	if err != nil {
		return fmt.Errorf("settle: %w", err)
	}

	// Announced only now, after the ledger has committed it.
	//
	// Broadcasting when the book matches would put trades on the tape that
	// settlement went on to drop, and a client cannot tell the difference — it
	// would be showing executions that never happened and that no REST read
	// would ever confirm.
	service.announce(tradeID, trade)

	return nil
}

// announce puts a settled trade on the public stream.
//
// The taker is the incoming order by definition — it is the one that crossed —
// so the side it came in on is the side that crossed the spread.
func (service *TradeService) announce(tradeID int64, trade models.Trade) {
	if service.Stream == nil {
		return
	}

	service.Stream.Publish(stream.Event{
		Type:   stream.EventTrade,
		Market: trade.Market,
		Payload: models.MarketTrade{
			ID:         tradeID,
			Market:     trade.Market,
			Quantity:   trade.Quantity,
			Price:      trade.Price,
			TakerSide:  trade.IncomingSide,
			ExecutedAt: trade.ExecutionTime,
		},
	})
}

// releaseCancelled returns what is left of a cancelled order's lock.
//
// Losing the race to a fill is not a failure: the order reached a terminal
// status first, the book had already matched it, and there is nothing left to
// release. The event is finished, so it reports success and the row closes.
func (service *TradeService) releaseCancelled(ctx context.Context, eventID int64, cancel models.OrderCancel) error {

	m, ok := service.MarketRegistry.BySymbol(cancel.Market)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownMarket, cancel.Market)
	}

	currency, err := m.Locks(cancel.Side)
	if err != nil {
		return err
	}

	err = service.WalletStore.CancelOrder(ctx, eventID, cancel.OrderID, currency.Code)
	if errors.Is(err, stores.ErrNotCancellable) {
		log.Printf("settlement: order %d filled before its cancellation arrived", cancel.OrderID)
		return nil
	}
	return err
}

// Default and maximum page sizes for the trade feeds. A caller asking for
// nothing sensible gets the default rather than an error, and no caller can ask
// for an unbounded scan.
const (
	DefaultTradeLimit = 50
	MaxTradeLimit     = 500
)

// ClampLimit keeps a client-supplied page size inside the bounds above.
func ClampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultTradeLimit
	case limit > MaxTradeLimit:
		return MaxTradeLimit
	default:
		return limit
	}
}

// FillsForUser returns the caller's own executions, newest first.
func (service *TradeService) FillsForUser(ctx context.Context, userID int64, limit int) (*models.Fills, error) {
	return service.TradeStore.FillsByUserID(ctx, userID, ClampLimit(limit))
}

// RecentTrades returns one market's public tape.
//
// The symbol is resolved through the registry first so an unknown market is a
// 404 rather than an empty tape, which would otherwise be indistinguishable
// from a real market that has never traded.
func (service *TradeService) RecentTrades(ctx context.Context, symbol string, limit int) (*models.MarketTrades, error) {
	m, ok := service.MarketRegistry.BySymbol(symbol)
	if !ok {
		return nil, ErrUnknownMarket
	}
	return service.TradeStore.RecentByMarket(ctx, m.Symbol, ClampLimit(limit))
}

// TickerWindowHours is the trailing window every ticker is reported over.
const TickerWindowHours = 24

// Ticker summarises one market over the trailing TickerWindowHours.
func (service *TradeService) Ticker(ctx context.Context, symbol string) (*models.Ticker, error) {
	m, ok := service.MarketRegistry.BySymbol(symbol)
	if !ok {
		return nil, ErrUnknownMarket
	}
	return service.TradeStore.TickerByMarket(ctx, m.Symbol, TickerWindowHours)
}

// Tickers summarises every listed market, in listing order.
func (service *TradeService) Tickers(ctx context.Context) ([]models.Ticker, error) {
	markets := service.MarketRegistry.Markets()

	out := make([]models.Ticker, 0, len(markets))
	for _, m := range markets {
		ticker, err := service.TradeStore.TickerByMarket(ctx, m.Symbol, TickerWindowHours)
		if err != nil {
			return nil, err
		}
		out = append(out, *ticker)
	}
	return out, nil
}
