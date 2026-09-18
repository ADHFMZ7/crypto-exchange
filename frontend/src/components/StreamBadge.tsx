import React from "react";
import type { StreamStatus } from "../hooks/useMarketStream";

const LABEL: Record<StreamStatus, string> = {
  live: "Live",
  connecting: "Connecting",
  offline: "Reconnecting"
};

const TITLE: Record<StreamStatus, string> = {
  live: "Executions are pushed as they happen",
  connecting: "Opening the live feed",
  offline: "The feed dropped — falling back to refreshing on a timer"
};

/**
 * Whether this panel is being pushed to or is asking.
 *
 * Worth showing because the two are not equivalent: on the feed a trade appears
 * the moment it settles, and on the fallback it can be a few seconds stale. A
 * user watching a price wants to know which one they are looking at.
 */
export const StreamBadge: React.FC<{ status: StreamStatus }> = ({ status }) => (
  <span className={`stream-badge stream-${status}`} title={TITLE[status]}>
    <span className="stream-dot" aria-hidden="true" />
    {LABEL[status]}
  </span>
);
