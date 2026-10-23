import type { ReactNode } from "react";
import type { Theme } from "../types";
import { repositoryUrl } from "../types";

export const Icon = ({ name }: { name: string }) => {
  const paths: Record<string, ReactNode> = {
    sun: <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></>,
    moon: <path d="M20.2 15.6A9 9 0 0 1 8.4 3.8a9 9 0 1 0 11.8 11.8Z" />,
    github: <path d="M12 2a10 10 0 0 0-3.2 19.5c.5.1.7-.2.7-.5v-1.9c-2.8.6-3.4-1.2-3.4-1.2-.4-1.1-1.1-1.4-1.1-1.4-.9-.6.1-.6.1-.6 1 .1 1.5 1 1.5 1 .9 1.6 2.4 1.1 3 .9.1-.7.3-1.1.6-1.4-2.2-.3-4.6-1.1-4.6-4.9 0-1.1.4-2 1-2.7-.1-.3-.4-1.3.1-2.6 0 0 .8-.3 2.8 1a9.7 9.7 0 0 1 5 0c1.9-1.3 2.8-1 2.8-1 .5 1.3.2 2.3.1 2.6.6.7 1 1.6 1 2.7 0 3.8-2.4 4.6-4.6 4.9.4.3.7.9.7 1.8V21c0 .3.2.6.7.5A10 10 0 0 0 12 2Z" />,
    arrow: <path d="m7 17 10-10M8 7h9v9" />,
    data: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v7c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12v7c0 1.7 3.6 3 8 3s8-1.3 8-3v-7" /></>,
    realtime: <path d="m13 2-9 12h7l-1 8 10-12h-7V2Z" />,
    rules: <><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z" /><path d="m9 12 2 2 4-4" /></>,
    users: <><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8M22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8" /></>,
    clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>,
    back: <path d="m15 18-6-6 6-6" />,
    plus: <path d="M12 5v14M5 12h14" />,
    trash: <><path d="M3 6h18M8 6V3h8v3M19 6l-1 15H6L5 6M10 11v5M14 11v5" /></>,
    check: <path d="m5 12 4 4L19 6" />,
  };
  return <svg viewBox="0 0 24 24" aria-hidden="true">{paths[name]}</svg>;
};

export function Logo({ onClick }: { onClick?: () => void }) {
  return (
    <button className="logo" onClick={onClick} type="button">
      <span className="logo-mark"><i>R</i></span>
      <strong>RIZZBASE</strong>
    </button>
  );
}

export function StarButton() {
  return <a className="star-button" href={repositoryUrl} target="_blank" rel="noreferrer"><Icon name="github" /> <b>GITHUB</b></a>;
}

export function ThemeToggle({ theme, toggle }: { theme: Theme; toggle: () => void }) {
  return (
    <button className="theme-toggle" onClick={toggle} type="button" aria-label={`Switch to ${theme === "light" ? "dark" : "light"} mode`}>
      <span className={theme === "light" ? "active" : ""}><Icon name="sun" /></span>
      <span className={theme === "dark" ? "active" : ""}><Icon name="moon" /></span>
      <i className={theme === "dark" ? "right" : ""} />
    </button>
  );
}

export function Button({ children, tone = "yellow", onClick }: { children: ReactNode; tone?: "yellow" | "pink" | "plain" | "black"; onClick?: () => void }) {
  return <button className={`btn ${tone}`} type="button" onClick={onClick}>{children}</button>;
}

export function Ticker() {
  return <div className="ticker"><div>NOW SERVING AURA <b>✦</b> RIZZBASE IS SO BACK <b>✦</b> NO CAP DATABASE <b>✦</b> NOW SERVING AURA <b>✦</b> RIZZBASE IS SO BACK <b>✦</b></div></div>;
}

export function ComingSoonNotice({ message }: { message: string }) {
  return (
    <aside className="coming-soon-notice" role="status">
      <span>PREVIEW ONLY</span>
      <p>{message}</p>
    </aside>
  );
}

export function BigBanner() {
  return <section className="big-banner"><div><small>LOCAL-FIRST V1</small><h2>YOUR PROJECT.<br /><span>YOUR DEVICE.</span></h2></div><a className="btn pink" href="https://www.npmjs.com/package/rizzbase" target="_blank" rel="noreferrer">GET THE SDK <Icon name="arrow" /></a></section>;
}
