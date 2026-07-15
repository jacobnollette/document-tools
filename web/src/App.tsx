import { Link, Outlet } from "react-router-dom";

export default function App() {
  return (
    <div className="app">
      <header className="app-header">
        <Link to="/" className="app-title">
          📄 Document Tools
        </Link>
      </header>
      <main className="app-main">
        <Outlet />
      </main>
    </div>
  );
}
