import { createContext, useContext } from "react";
import type { User } from "./api";

export interface Session {
  user: User | null;
  setUser: (user: User | null) => void;
  refresh: () => Promise<void>;
}

export const SessionContext = createContext<Session>({
  user: null,
  setUser: () => {},
  refresh: async () => {},
});

export function useSession(): Session {
  return useContext(SessionContext);
}
