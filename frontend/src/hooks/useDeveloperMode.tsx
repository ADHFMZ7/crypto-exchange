import React, { createContext, useContext, useEffect, useMemo, useState } from "react";

/**
 * Whether to show how the app is wired, as well as what it does.
 *
 * This app was built alongside the backend it talks to, and every panel
 * declares which endpoint feeds it so the integration gap is visible while
 * running rather than buried in a README. That is genuinely useful while
 * endpoints are still landing, and it is noise to somebody who just wants to
 * place an order — so it is a mode rather than a permanent fixture.
 *
 * Off by default. On, panels regain their endpoint badges, their provenance
 * colouring and the notes explaining what happens underneath, and the backend
 * coverage board reappears.
 */

type DeveloperContextValue = {
  developer: boolean;
  toggle: () => void;
};

const DeveloperContext = createContext<DeveloperContextValue | undefined>(undefined);

const STORAGE_KEY = "crypto-exchange-developer";

export const DeveloperProvider: React.FC<React.PropsWithChildren> = ({ children }) => {
  const [developer, setDeveloper] = useState<boolean>(
    () => localStorage.getItem(STORAGE_KEY) === "on"
  );

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, developer ? "on" : "off");
  }, [developer]);

  const value = useMemo(
    () => ({ developer, toggle: () => setDeveloper((prev) => !prev) }),
    [developer]
  );

  return <DeveloperContext.Provider value={value}>{children}</DeveloperContext.Provider>;
};

export const useDeveloperMode = (): DeveloperContextValue => {
  const ctx = useContext(DeveloperContext);
  if (!ctx) {
    throw new Error("useDeveloperMode must be used within DeveloperProvider");
  }
  return ctx;
};
