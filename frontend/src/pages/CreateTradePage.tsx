import React, { useCallback, useEffect, useMemo, useState } from "react";
import { MarketHeader } from "../components/MarketHeader";
import { OrderBook } from "../components/OrderBook";
import { OrderTicket } from "../components/OrderTicket";
import { TradeTape } from "../components/TradeTape";
import { useAuth } from "../hooks/useAuth";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { ApiError, api } from "../lib/api";
import type { OrderbookSnapshot, WalletBalance } from "../types";

/**
 * The trade screen.
 *
 * One viewport, three tiles of equal width: what you are willing to pay, what
 * is resting, and what has just traded. They fill the height together and
 * scroll their own bodies, so the page itself never scrolls and nothing sits
 * beside dead space.
 *
 * The book earns a third of the screen because this exchange has no external
 * price feed — what is resting is the only evidence of what a limit is worth,
 * so it belongs next to the field where that number is typed.
 */
export const CreateTradePage: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();

  const [symbol, setSymbol] = useState<string | undefined>(reference.markets[0]?.symbol);
  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [book, setBook] = useState<OrderbookSnapshot>();

  const market = useMemo(
    () => reference.markets.find((m) => m.symbol === symbol),
    [reference.markets, symbol]
  );

  // Reference data can arrive after first paint and change the market list.
  useEffect(() => {
    if (!market && reference.markets.length) setSymbol(reference.markets[0].symbol);
  }, [market, reference.markets]);

  const loadWallet = useCallback(async () => {
    if (!token) return;
    try {
      const wallet = await api.getWallet(token);
      setBalances(wallet.balances ?? []);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) logout();
    }
  }, [logout, token]);

  // Balances move without the user acting: a resting order's funds shift from
  // available to locked, and settlement moves them again when it fills.
  usePolling(loadWallet, 6000, Boolean(token));

  return (
    <div className="trade-screen">
      <MarketHeader symbol={symbol}>
        {reference.markets.length > 1 && (
          <select
            value={symbol ?? ""}
            onChange={(e) => setSymbol(e.target.value)}
            aria-label="Market"
            style={{ width: "auto" }}
          >
            {reference.markets.map((m) => (
              <option key={m.symbol} value={m.symbol}>
                {m.symbol}
              </option>
            ))}
          </select>
        )}
      </MarketHeader>

      <div className="trade-tiles">
        <OrderTicket
          market={market}
          balances={balances}
          bestBid={book?.bids[0]?.price}
          bestAsk={book?.asks[0]?.price}
          onPlaced={loadWallet}
        />
        <OrderBook symbol={symbol} onSnapshot={setBook} />
        <TradeTape symbol={symbol} />
      </div>
    </div>
  );
};
