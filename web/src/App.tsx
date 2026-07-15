import { useCallback, useEffect, useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { ApiError, getSetupStatus, logout, me, type User } from "./api";
import { SessionContext } from "./session";

// App gates everything behind install + login state, WordPress-style:
// no config → /setup; config but no session → /login; otherwise the app.
export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [checked, setChecked] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();

  const check = useCallback(async () => {
    try {
      const status = await getSetupStatus();
      if (!status.installed) {
        navigate("/setup", { replace: true });
        return;
      }
      setUser(await me());
      if (location.pathname === "/login" || location.pathname === "/setup") {
        navigate("/", { replace: true });
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setUser(null);
        if (location.pathname !== "/login") navigate("/login", { replace: true });
      }
    } finally {
      setChecked(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navigate]);

  useEffect(() => {
    void check();
  }, [check]);

  async function handleLogout() {
    await logout();
    setUser(null);
    navigate("/login");
  }

  return (
    <SessionContext.Provider value={{ user, setUser, refresh: check }}>
      <div className="app">
        <header className="app-header">
          <Link to="/" className="app-title">
            📄 Document Tools
          </Link>
          {user && (
            <div className="app-user">
              <span>{user.username}</span>
              <button className="btn btn-ghost" onClick={() => void handleLogout()}>
                Sign out
              </button>
            </div>
          )}
        </header>
        <main className="app-main">{checked ? <Outlet /> : <p>Loading…</p>}</main>
      </div>
    </SessionContext.Provider>
  );
}
