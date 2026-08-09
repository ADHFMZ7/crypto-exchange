import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { ApiError, api, errorMessage } from "../lib/api";
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
      setUser(await api.getMe(token));
    } catch (err) {
      // Only a rejected token should end the session. A backend that is down or
      // unreachable must not silently sign the user out.
      if (err instanceof ApiError && err.isUnauthorized) {
        setError("Session expired, please log in again.");
        logout();
        return;
      }
      setError(errorMessage(err));
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
        const { token: issued } = await api.login(email, password);
        setToken(issued);
        localStorage.setItem(STORAGE_KEY, issued);
        setUser(await api.getMe(issued));
        return true;
      } catch (err) {
        setError(errorMessage(err));
        logout();
        return false;
      } finally {
        setLoading(false);
      }
    },
    [logout]
  );

  const signup = useCallback(
    async (email: string, fullname: string, password: string) => {
      setLoading(true);
      setError(undefined);

      try {
        await api.signup(email, fullname, password);
      } catch (err) {
        setError(errorMessage(err));
        setLoading(false);
        return false;
      }

      setLoading(false);
      // Land the new account straight in the app.
      return login(email, password);
    },
    [login]
  );

  const value = useMemo(
    () => ({ user, token, loading, error, login, signup, logout, refreshUser }),
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
