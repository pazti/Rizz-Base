import type { ReactNode } from "react";
import type { Go, PageProps, Theme } from "../types";
import { repositoryUrl } from "../types";
import { ComingSoonNotice, Icon, Logo, StarButton, ThemeToggle } from "./ui";

export function Header({ theme, toggle, go }: PageProps) {
  return (
    <header className="site-header">
      <Logo onClick={() => go("home")} onDark={theme === "dark"} />
      <nav>
        <button onClick={() => go("home")}>FEATURES</button>
        <button onClick={() => go("studio")}>STUDIO PREVIEW</button>
        <button onClick={() => go("docs")}>DOCS</button>
        <button onClick={() => go("pricing")}>PRICING</button>
      </nav>
      <div className="header-actions">
        <StarButton />
        <ThemeToggle theme={theme} toggle={toggle} />
      </div>
    </header>
  );
}

export function Footer({ go }: { go: Go }) {
  return <footer><Logo onClick={() => go("home")} onDark /><p>© 2026 Rizzbase. Made with zero chill.</p><div><a href="/privacy/">Privacy</a><a href={repositoryUrl} target="_blank" rel="noreferrer">GitHub</a><span>LOCAL-FIRST V1</span></div></footer>;
}

export function Sidebar({ active, go, theme }: { active: string; go: Go; theme: Theme }) {
  return (
    <aside className="app-sidebar">
      <Logo onClick={() => go("home")} onDark={theme === "dark"} />
      <div className="workspace"><i>RB</i><span><b>Local Project</b><small>PRIVATE FILE</small></span><b>⌄</b></div>
      <nav>
        <small>WORKSPACE</small>
        <button className={active === "data" ? "active" : ""} onClick={() => go("dashboard")}><Icon name="data" /> Data</button>
        <button onClick={() => { window.location.hash = "/docs/cli"; }}><Icon name="data" /> Queries</button>
        <button onClick={() => { window.location.hash = "/docs/security"; }}><Icon name="rules" /> Encryption</button>
        <button onClick={() => go("docs")}><Icon name="rules" /> Docs</button>
      </nav>
      <div className="side-bottom"><button onClick={() => go("docs")}>? <span>Need help, bestie?</span></button><div><i>RB</i><span><b>Rizzbase</b><small>Local preview</small></span></div></div>
    </aside>
  );
}

export function AppShell({ children, page, theme, toggle, go }: { children: ReactNode; page: string; theme: Theme; toggle: () => void; go: Go }) {
  return (
    <div className="app-shell">
      <Sidebar active="data" go={go} theme={theme} />
      <div className="app-area">
        <header className="app-topbar"><div><button onClick={() => go("dashboard")}>PREVIEW</button><span>/</span><b>{page}</b></div><div><span className="sync">SAMPLE DATA</span><ThemeToggle theme={theme} toggle={toggle} /></div></header>
        <ComingSoonNotice message="Preview only: sample data, not connected to local .rizz files. For real data, install the engine first, then use the Go CLI, Node.js SDK, or Python SDK. See Docs → Installation." />
        {children}
      </div>
    </div>
  );
}
