import React, { createContext, useContext, useEffect, useState } from "react";
import { loadReference, type ReferenceData } from "../lib/reference";

/**
 * Loads currency and market metadata once at startup and shares it app-wide.
 *
 * The provider renders a status screen instead of its children until the load
 * settles, which is deliberate. Exponents decide what every amount in the app
 * means: a balance formatted without them is wrong by a factor of 10^exponent,
 * and an order built without them is that same error on the wire. There is no
 * useful partial render, so there is no reason to offer one.
 *
 * Gating here also keeps `useReference()` returning a plain ReferenceData rather
 * than something nullable, so no consumer has to defend against absent
 * exponents — by the time any of them mounts, the data exists.
 */

type ReferenceContextValue = {
  reference: ReferenceData;
};

const ReferenceContext = createContext<ReferenceContextValue | undefined>(undefined);

type State =
  | { status: "loading" }
  | { status: "ready"; reference: ReferenceData }
  | { status: "error"; message: string };

export const ReferenceProvider: React.FC<React.PropsWithChildren> = ({ children }) => {
  const [state, setState] = useState<State>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });

    loadReference()
      .then((reference) => {
        if (!cancelled) setState({ status: "ready", reference });
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setState({
          status: "error",
          message: err instanceof Error ? err.message : "Could not load reference data."
        });
      });

    return () => {
      cancelled = true;
    };
  }, [attempt]);

  if (state.status === "loading") {
    return <ReferenceSplash title="Loading exchange data…" />;
  }

  if (state.status === "error") {
    return (
      <ReferenceSplash
        title="Exchange data unavailable"
        message={state.message}
        onRetry={() => setAttempt((n) => n + 1)}
      />
    );
  }

  return (
    <ReferenceContext.Provider value={{ reference: state.reference }}>
      {children}
    </ReferenceContext.Provider>
  );
};

type SplashProps = {
  title: string;
  message?: string;
  onRetry?: () => void;
};

/**
 * Deliberately blunt about what is missing. The currencies and markets endpoints
 * are public and unauthenticated, so failing to read them means the API is
 * unreachable or misconfigured — not that the user needs to sign in.
 */
const ReferenceSplash: React.FC<SplashProps> = ({ title, message, onRetry }) => (
  <main className="container">
    <section className="panel" style={{ marginTop: 48, maxWidth: 620 }}>
      <div className="headline">
        <div>
          <div className="tag">Reference data</div>
          <h2 className="panel-title">{title}</h2>
        </div>
      </div>

      {message && <div className="pill status-danger">{message}</div>}

      <div className="muted" style={{ marginTop: 12 }}>
        Currency exponents come from <code>GET /currencies</code> and decide what every amount here
        means, so the app will not render balances or accept an order without them.
      </div>

      {onRetry && (
        <div style={{ marginTop: 16 }}>
          <button type="button" onClick={onRetry}>
            Try again
          </button>
        </div>
      )}
    </section>
  </main>
);

export const useReference = (): ReferenceContextValue => {
  const ctx = useContext(ReferenceContext);
  if (!ctx) {
    throw new Error("useReference must be used within a ReferenceProvider");
  }
  return ctx;
};
