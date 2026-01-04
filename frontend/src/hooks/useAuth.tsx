import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { User } from "../types";

type AuthContextValue = {
  user: User | null;
  token: string | null;
  loading: boolean;
  error?: string;
  login: (email: string, password: string) => Promise<boolean>;
  signup: (email: string, fullname: string, password: string) => Promise<boolean>;
  logout: () => void;
  refreshUser: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";
const STORAGE_KEY = "crypto-exchange-token";

export const AuthProvider: React.FC<React.PropsWithChildren> = ({ children }) => {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem(STORAGE_KEY));
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  const logout = useCallback(() => {
    setToken(null);
    setUser(null);
    localStorage.removeItem(STORAGE_KEY);
  }, []);

  const refreshUser = useCallback(async () => {
    if (!token) {
      setUser(null);
      return;
    }

    setLoading(true);
    setError(undefined);
    try {
      const res = await fetch(`${API_BASE}/users/me`, {
        headers: {
          Authorization: `Bearer ${token}`
        }
      });

      if (!res.ok) {
        throw new Error(`Unable to fetch user: ${res.status}`);
      }

      const data: User = await res.json();
      setUser(data);
    } catch (err) {
      console.error(err);
      setUser(null);
      setError("Session expired, please log in again.");
      logout();
    } finally {
      setLoading(false);
    }
  }, [logout, token]);

  useEffect(() => {
    if (token) {
      refreshUser();
    }
  }, [token, refreshUser]);

  const login = useCallback(
    async (email: string, password: string) => {
      setLoading(true);
      setError(undefined);

      try {
        const res = await fetch(`${API_BASE}/auth/login`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json"
          },
          body: JSON.stringify({ email, password })
        });

        if (!res.ok) {
          const message = await res.text();
          throw new Error(message || "Login failed");
        }

        const data = (await res.json()) as { token: string };
        setToken(data.token);
        localStorage.setItem(STORAGE_KEY, data.token);
        await refreshUser();
        return true;
      } catch (err) {
        console.error(err);
        setError(err instanceof Error ? err.message : "Login failed");
        logout();
        return false;
      } finally {
        setLoading(false);
      }
    },
    [logout, refreshUser]
  );

  const signup = useCallback(
    async (email: string, fullname: string, password: string) => {
      setLoading(true);
      setError(undefined);

      try {
        const res = await fetch(`${API_BASE}/users`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json"
          },
          body: JSON.stringify({ email, fullname, password })
        });

        if (!res.ok) {
          const message = await res.text();
          throw new Error(message || "Signup failed");
        }

        // After signup, attempt a login so the user lands in the app
        return await login(email, password);
      } catch (err) {
        console.error(err);
        setError(err instanceof Error ? err.message : "Signup failed");
        return false;
      } finally {
        setLoading(false);
      }
    },
    [login]
  );

  const value = useMemo(
    () => ({
      user,
      token,
      loading,
      error,
      login,
      signup,
      logout,
      refreshUser
    }),
    [error, loading, login, logout, refreshUser, signup, token, user]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = (): AuthContextValue => {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
};
