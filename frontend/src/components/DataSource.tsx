import React from "react";
import { useDeveloperMode } from "../hooks/useDeveloperMode";

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
  /** Shown to everyone. Say what the panel is for, not how it is fed. */
  note?: React.ReactNode;
  /**
   * Shown only in developer mode: what happens underneath, which endpoint
   * answers, why a number rounds the way it does.
   */
  devNote?: React.ReactNode;
  actions?: React.ReactNode;
  /**
   * Stretch to the height of the grid cell and scroll the body rather than the
   * page. For tiled layouts where every panel should end on the same line.
   */
  fill?: boolean;
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
  devNote,
  actions,
  fill = false,
  children
}) => {
  const { developer } = useDeveloperMode();

  // Provenance colouring says where numbers come from, which is a question
  // only somebody building this app is asking.
  const provenance = developer ? ` panel-${kind}` : "";

  return (
    <section className={`panel${provenance}${fill ? " panel-fill" : ""}`}>
      <div className="headline">
        <div>
          {eyebrow && <div className="tag">{eyebrow}</div>}
          <h2 className="panel-title">{title}</h2>
        </div>
        <div className="inline-actions">
          {actions}
          {developer && <SourceBadge kind={kind} endpoint={endpoint} />}
        </div>
      </div>
      {note && <SourceNote kind={developer ? kind : "live"}>{note}</SourceNote>}
      {developer && devNote && <SourceNote kind={kind}>{devNote}</SourceNote>}
      {fill ? <div className="panel-body">{children}</div> : children}
    </section>
  );
};
