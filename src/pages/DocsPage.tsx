import type { PageProps } from "../types";
import { Footer } from "../components/layout";
import { DocsContent } from "../Docs.tsx";
import { Logo, StarButton, ThemeToggle } from "../components/ui";

export default function DocsPage({ theme, toggle, go, slug }: PageProps & { slug: string }) {
  return (
    <>
      <header className="docs-topbar">
        <Logo onClick={() => go("home")} onDark={theme === "dark"} />
        <div>
          <StarButton />
          <ThemeToggle theme={theme} toggle={toggle} />
        </div>
      </header>
      <main className="docs-full">
        <DocsContent slug={slug} onSelect={(next) => { window.location.hash = `/docs/${next}`; }} />
      </main>
      <Footer go={go} />
    </>
  );
}
