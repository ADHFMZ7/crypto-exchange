import React, { useState } from "react";
import { Link, useNavigate, useLocation } from "react-router-dom";
import type { Location } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";

type AuthMode = "login" | "signup";

export const AuthPage: React.FC<{ mode?: AuthMode }> = ({ mode = "login" }) => {
  const [formMode, setFormMode] = useState<AuthMode>(mode);
  const [email, setEmail] = useState("");
  const [fullname, setFullname] = useState("");
  const [password, setPassword] = useState("");
  const navigate = useNavigate();
  const location = useLocation();
  const { login, signup, loading, error } = useAuth();

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const next = formMode === "login" ? await login(email, password) : await signup(email, fullname, password);
    if (next) {
      const redirectTo = (location.state as { from?: Location })?.from?.pathname ?? "/";
      navigate(redirectTo, { replace: true });
    }
  };

  const toggleMode = () => {
    setFormMode(formMode === "login" ? "signup" : "login");
  };

  return (
    <div className="panel" style={{ maxWidth: 480, margin: "60px auto" }}>
      <div className="headline">
        <div>
          <div className="tag">Access</div>
          <h2 style={{ margin: "6px 0" }}>{formMode === "login" ? "Welcome back" : "Create an account"}</h2>
          <div className="muted">
            {formMode === "login"
              ? "Log in to see your balances and place trades."
              : "Sign up to start trading. You will get a starter USD balance from the backend."}
          </div>
        </div>
        <button type="button" onClick={toggleMode} className="ghost-button">
          {formMode === "login" ? "Need an account?" : "Have an account?"}
        </button>
      </div>

      <form className="stack" onSubmit={onSubmit}>
        <label className="stack">
          <span>Email</span>
          <input
            type="email"
            required
            value={email}
            placeholder="you@example.com"
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>

        {formMode === "signup" && (
          <label className="stack">
            <span>Full name</span>
            <input
              type="text"
              required
              value={fullname}
              placeholder="Satoshi Nakamoto"
              onChange={(e) => setFullname(e.target.value)}
            />
          </label>
        )}

        <label className="stack">
          <span>Password</span>
          <input
            type="password"
            required
            value={password}
            placeholder="••••••••"
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>

        {error && <div className="pill status-danger">{error}</div>}

        <button type="submit" disabled={loading}>
          {loading ? "Working..." : formMode === "login" ? "Login" : "Sign up"}
        </button>
      </form>

      <div style={{ marginTop: 12 }} className="muted">
        <Link to={formMode === "login" ? "/signup" : "/login"}>Switch to {formMode === "login" ? "signup" : "login"}</Link>
      </div>
    </div>
  );
};
