import React from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./components/Layout";
import { ProtectedRoute } from "./components/ProtectedRoute";
import { AuthProvider, useAuth } from "./hooks/useAuth";
import { ThemeProvider } from "./hooks/useTheme";
import { AuthPage } from "./pages/AuthPage";
import { CreateTradePage } from "./pages/CreateTradePage";
import { HomePage } from "./pages/HomePage";
import { TradesPage } from "./pages/TradesPage";
import { WalletPage } from "./pages/WalletPage";

const RoutedApp: React.FC = () => {
  const { token } = useAuth();

  return (
    <Layout>
      <Routes>
        <Route path="/login" element={<AuthPage mode="login" />} />
        <Route path="/signup" element={<AuthPage mode="signup" />} />

        <Route
          path="/"
          element={
            <ProtectedRoute>
              <HomePage />
            </ProtectedRoute>
          }
        />
        <Route
          path="/trades/new"
          element={
            <ProtectedRoute>
              <CreateTradePage />
            </ProtectedRoute>
          }
        />
        <Route
          path="/wallet"
          element={
            <ProtectedRoute>
              <WalletPage />
            </ProtectedRoute>
          }
        />
        <Route
          path="/trades"
          element={
            <ProtectedRoute>
              <TradesPage />
            </ProtectedRoute>
          }
        />

        <Route path="*" element={<Navigate to={token ? "/" : "/login"} replace />} />
      </Routes>
    </Layout>
  );
};

const App: React.FC = () => {
  return (
    <ThemeProvider>
      <AuthProvider>
        <BrowserRouter>
          <RoutedApp />
        </BrowserRouter>
      </AuthProvider>
    </ThemeProvider>
  );
};

export default App;
