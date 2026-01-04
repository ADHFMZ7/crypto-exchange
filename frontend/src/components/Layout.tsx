import React from "react";
import { Link, NavLink } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { useTheme } from "../hooks/useTheme";

type LayoutProps = {
  children: React.ReactNode;
};

const links = [
  { to: "/", label: "Home", end: true },
  { to: "/wallet", label: "Wallet" },
  { to: "/trades/new", label: "New Trade" }
];

export const Layout: React.FC<LayoutProps> = ({ children }) => {
  const { user, logout } = useAuth();
  const { theme, toggle } = useTheme();

  return (
    <div className="app-shell">
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

      <main>{children}</main>
    </div>
  );
};
