import React from "react";
import { SourcedPanel } from "./DataSource";
import { useReference } from "../hooks/useReference";
import { effectiveExponent } from "../lib/markets";

/**
 * What the exchange lists, and what each currency's precision means.
 *
 * It lives beside the other status panels rather than on the trade screen: the
 * ticket already states each field's precision inline ("up to 8 decimals"),
 * and this is the reference someone consults, not something they read while
 * placing an order.
 */
export const ReferenceStatus: React.FC = () => {
  const { reference } = useReference();

  return (
    <SourcedPanel
      eyebrow="Reference"
      title="Markets and precision"
      kind="live"
      endpoint="GET /currencies · GET /markets"
      note={
        <>
          Served by the backend, which is the authority on both. Nothing here is hardcoded in the
          frontend — an exponent decides what every amount in the app means, so a second copy that
          disagreed would be a factor-of-10<sup>n</sup> error in all of them.
        </>
      }
    >
      <table className="table">
        <thead>
          <tr>
            <th>Market</th>
            <th>Base — order amounts</th>
            <th>Quote — prices and totals</th>
            <th style={{ textAlign: "right" }}>Amounts are</th>
          </tr>
        </thead>
        <tbody>
          {reference.markets.map((market) => (
            <tr key={market.symbol}>
              <td>
                <code>{market.symbol}</code>
              </td>
              <td>
                {market.base}{" "}
                <span className="muted">({effectiveExponent(reference, market.base)}dp)</span>
              </td>
              <td>
                {market.quote}{" "}
                <span className="muted">({effectiveExponent(reference, market.quote)}dp)</span>
              </td>
              <td style={{ textAlign: "right" }} className="muted">
                {effectiveExponent(reference, market.base) === 0 ? "whole units" : "minor units"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </SourcedPanel>
  );
};
