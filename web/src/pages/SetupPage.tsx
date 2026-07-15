import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { getSetupStatus, install, type DatabaseSettings } from "../api";
import { useSession } from "../session";

const emptyDb: DatabaseSettings = {
  host: "localhost",
  port: "5432",
  user: "",
  password: "",
  name: "documents",
  sslmode: "disable",
};

// First-run installer. Database fields are pre-filled from server-side
// environment variables when set, and everything stays editable.
export default function SetupPage() {
  const [db, setDb] = useState<DatabaseSettings>(emptyDb);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { setUser } = useSession();

  useEffect(() => {
    getSetupStatus()
      .then((status) => {
        if (status.installed) {
          navigate("/", { replace: true });
        } else if (status.defaults) {
          setDb(status.defaults);
        }
      })
      .catch(() => {});
  }, [navigate]);

  function field(key: keyof DatabaseSettings, label: string, type = "text") {
    return (
      <label className="form-field">
        <span>{label}</span>
        <input
          type={type}
          value={db[key]}
          onChange={(e) => setDb({ ...db, [key]: e.target.value })}
          required={key !== "password"}
        />
      </label>
    );
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      setError("Passwords do not match.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const result = await install(db, { username, password });
      setUser(result.user);
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "installation failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="panel">
      <h1>Welcome</h1>
      <p className="hint-text">
        Let's set up your document library. Connect a PostgreSQL database and
        create your account — the first account is created right here.
      </p>

      <form onSubmit={(e) => void handleSubmit(e)}>
        <h2>Database</h2>
        <div className="form-grid">
          {field("host", "Host")}
          {field("port", "Port")}
          {field("name", "Database name")}
          {field("user", "Database user")}
          {field("password", "Database password", "password")}
          <label className="form-field">
            <span>SSL mode</span>
            <select
              value={db.sslmode}
              onChange={(e) => setDb({ ...db, sslmode: e.target.value })}
            >
              <option value="disable">disable</option>
              <option value="prefer">prefer</option>
              <option value="require">require</option>
              <option value="verify-full">verify-full</option>
            </select>
          </label>
        </div>

        <h2>Your account</h2>
        <div className="form-grid">
          <label className="form-field">
            <span>Username</span>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              required
            />
          </label>
          <label className="form-field">
            <span>Password (min. 8 characters)</span>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              minLength={8}
              required
            />
          </label>
          <label className="form-field">
            <span>Confirm password</span>
            <input
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              autoComplete="new-password"
              required
            />
          </label>
        </div>

        {error && <p className="error-text">{error}</p>}
        <button className="btn btn-primary" disabled={busy} type="submit">
          {busy ? "Installing…" : "Install"}
        </button>
      </form>
    </div>
  );
}
