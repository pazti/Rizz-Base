import type { PageProps } from "../types";
import { Button, Icon, Logo, ThemeToggle } from "../components/ui";

export default function Auth({ theme, toggle, go }: PageProps) {
  return (
    <main className="auth-page">
      <div className="auth-top"><Logo onClick={() => go("home")} /><ThemeToggle theme={theme} toggle={toggle} /></div>
      <div className="auth-scribble one">✦</div><div className="auth-scribble two">↗</div>
      <section className="auth-card coming-soon-card">
        <div className="auth-badge">LOCAL-ONLY V1</div>
        <h1>THE UI IS A<br /><span>PREVIEW ONLY.</span></h1>
        <p>Accounts and sign-in are not part of Rizzbase V1. This product interface is still a preview and does not connect to the local engine.</p>
        <Button tone="plain" onClick={() => go("home")}>BACK TO HOME <Icon name="arrow" /></Button>
        <small>For local Project files, install the engine and use the Go CLI, Node.js SDK, or Python SDK.</small>
      </section>
      <div className="auth-quote">“local files. encrypted. no cloud drama.”<b>— Rizzbase V1</b></div>
    </main>
  );
}
