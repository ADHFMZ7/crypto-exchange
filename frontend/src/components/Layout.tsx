import React from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { useDeveloperMode } from "../hooks/useDeveloperMode";
import { useTheme } from "../hooks/useTheme";

type LayoutProps = {
  children: React.ReactNode;
};

// "Trades" and "New Trade" sat next to each other meaning quite different
// things. Trade is where you place one; Orders is what you have placed.
const links = [
  { to: "/", label: "Home", end: true },
  { to: "/markets", label: "Markets" },
  { to: "/trades/new", label: "Trade" },
  { to: "/trades", label: "Orders", end: true },
  { to: "/wallet", label: "Wallet" }
];

export const Layout: React.FC<LayoutProps> = ({ children }) => {
  const { user, logout } = useAuth();
  const { theme, toggle } = useTheme();
  const { developer, toggle: toggleDeveloper } = useDeveloperMode();
  const { pathname } = useLocation();

  // The trade screen is a dashboard, not a document: its panels tile to fill
  // one viewport and scroll their own bodies. Every other route reads
  // top-to-bottom and keeps the normal page scroll and the narrower measure.
  const filling = pathname === "/trades/new";

  return (
    <div className={`app-shell${filling ? " app-shell-fill" : ""}`}>
      <header className="panel nav">
        <Link className="brand" to="/">
          Crypto Exchange
        </Link>

        {user && (
          <nav className="nav-links">
            {links.map((link) => (
              <NavLink
                key={link.to}
                to={link.to}
                end={Boolean(link.end)}
                className={({ isActive }) => `pill${isActive ? " " : ""}`}
                style={({ isActive }) => ({
                  backgroundColor: isActive ? "rgba(34, 211, 238, 0.12)" : undefined,
                  borderColor: isActive ? "var(--accent)" : "var(--border)"
                })}
              >
                {link.label}
              </NavLink>
            ))}
          </nav>
        )}

        <div className="inline-actions">
          {user ? (
            <>
              <div className="pill">
                <strong>{user.fullname}</strong>
                <div className="muted">{user.email}</div>
              </div>
              <button type="button" onClick={logout}>
                Logout
              </button>
              <button
                type="button"
                onClick={toggleDeveloper}
                className={`icon-button${developer ? " icon-button-on" : ""}`}
                aria-pressed={developer}
                title={
                  developer
                    ? "Hide endpoints and implementation notes"
                    : "Show which endpoint feeds each panel"
                }
                aria-label="Developer mode"
              >
                <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M8 6l-5 6 5 6" />
                  <path d="M16 6l5 6-5 6" />
                </svg>
              </button>
              <button
                type="button"
                onClick={toggle}
                className="icon-button"
                aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
              >
                {theme === "dark" ? "☀️" : "🌙"}
              </button>
            </>
          ) : (
            <>
              <div className="muted">Guest</div>
              <button
                type="button"
                onClick={toggleDeveloper}
                className={`icon-button${developer ? " icon-button-on" : ""}`}
                aria-pressed={developer}
                title={
                  developer
                    ? "Hide endpoints and implementation notes"
                    : "Show which endpoint feeds each panel"
                }
                aria-label="Developer mode"
              >
                <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M8 6l-5 6 5 6" />
                  <path d="M16 6l5 6-5 6" />
                </svg>
              </button>
              <button
                type="button"
                onClick={toggle}
                className="icon-button"
                aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
              >
                {theme === "dark" ? "☀️" : "🌙"}
              </button>
            </>
          )}
        </div>
      </header>

      <main className={filling ? "app-main-fill" : undefined}>{children}</main>
    </div>
  );
};
