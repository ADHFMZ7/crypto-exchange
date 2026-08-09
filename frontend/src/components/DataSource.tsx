import React from "react";

/**
 * Every panel in this app declares where its numbers come from, so nobody has to
 * guess whether a figure is real.
 *
 *   live  — served by an endpoint the Go backend implements today
 *   local — real actions you took, but stored only in this browser because the
 *           backend has no read endpoint for them yet
 *   mock  — fabricated placeholder, waiting on backend work
 */
export type SourceKind = "live" | "local" | "mock";

const LABEL: Record<SourceKind, string> = {
  live: "Live",
  local: "Browser only",
  mock: "Dummy data"
};

const DOT: Record<SourceKind, string> = {
  live: "●",
  local: "◐",
  mock: "○"
};

type SourceBadgeProps = {
  kind: SourceKind;
  /** The endpoint backing this data, or the one it is waiting on. */
  endpoint?: string;
};

export const SourceBadge: React.FC<SourceBadgeProps> = ({ kind, endpoint }) => (
  <span className={`tag source-badge source-${kind}`} title={endpoint}>
    <span aria-hidden="true">{DOT[kind]}</span>
    {LABEL[kind]}
    {endpoint && <code className="source-endpoint">{endpoint}</code>}
  </span>
);

type SourceNoteProps = {
  kind: SourceKind;
  children: React.ReactNode;
};

/** The one-line explanation that sits under a panel heading. */
export const SourceNote: React.FC<SourceNoteProps> = ({ kind, children }) => (
  <div className={`source-note source-note-${kind}`}>{children}</div>
);

type PanelProps = {
  title: string;
  eyebrow?: string;
  kind: SourceKind;
  endpoint?: string;
  note?: React.ReactNode;
  actions?: React.ReactNode;
  children: React.ReactNode;
};

/**
 * A panel that cannot be rendered without stating its data provenance —
 * `kind` is a required prop by design.
 */
export const SourcedPanel: React.FC<PanelProps> = ({
  title,
  eyebrow,
  kind,
  endpoint,
  note,
  actions,
  children
}) => (
  <section className={`panel panel-${kind}`}>
    <div className="headline">
      <div>
        {eyebrow && <div className="tag">{eyebrow}</div>}
        <h2 className="panel-title">{title}</h2>
      </div>
      <div className="inline-actions">
        {actions}
        <SourceBadge kind={kind} endpoint={endpoint} />
      </div>
    </div>
    {note && <SourceNote kind={kind}>{note}</SourceNote>}
    {children}
  </section>
);
