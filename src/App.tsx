import { useEffect, useState } from "react"
import type { Page, Theme } from "./types"
import { docs as docEntries } from "./docs"
import Landing from "./pages/Landing"
import { Dashboard, Editor, Events, Lobby } from "./pages/Preview"
import Auth from "./pages/Auth"
import Pricing from "./pages/Pricing"
import DocsPage from "./pages/DocsPage"
import Studio from "./Studio"
import Packages from "./pages/Packages"

export default function App() {
  if (import.meta.env.MODE === "studio") return <Studio />

  const getPage = (): Page => {
    const hash = window.location.hash.replace("#/", "")
    const root = hash.split("/")[0] as Page
    return [
      "home",
      "dashboard",
      "lobby",
      "editor",
      "events",
      "auth",
      "pricing",
      "docs",
      "studio",
      "packages",
    ].includes(root)
      ? root
      : "home"
  }
  const getDocSlug = (): string => {
    const parts = window.location.hash.replace("#/", "").split("/")
    const slug = parts[1] ?? ""
    return docEntries.some((d) => d.slug === slug) ? slug : "overview"
  }
  const [page, setPage] = useState<Page>(getPage)
  const [docSlug, setDocSlug] = useState<string>(getDocSlug)
  const [theme, setTheme] = useState<Theme>(
    () => localStorage.getItem("rizz-theme") as Theme || "light",
  )

  useEffect(() => {
    const sync = () => {
      setPage(getPage())
      setDocSlug(getDocSlug())
    }
    window.addEventListener("hashchange", sync)
    return () => window.removeEventListener("hashchange", sync)
  }, [])

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem("rizz-theme", theme)
  }, [theme])

  const go = (next: Page) => {
    window.location.hash = next === "docs" ? "/docs/overview" : `/${next}`
    setPage(next)
    if (next === "docs") setDocSlug("overview")
    window.scrollTo({ top: 0, behavior: "smooth" })
  }
  const toggle = () => setTheme(theme === "light" ? "dark" : "light")

  if (page === "dashboard")
    return <Dashboard theme={theme} toggle={toggle} go={go} />
  if (page === "lobby") return <Lobby theme={theme} toggle={toggle} go={go} />
  if (page === "editor") return <Editor theme={theme} toggle={toggle} go={go} />
  if (page === "events") return <Events theme={theme} toggle={toggle} go={go} />
  if (page === "auth") return <Auth theme={theme} toggle={toggle} go={go} />
  if (page === "pricing")
    return <Pricing theme={theme} toggle={toggle} go={go} />
  if (page === "docs")
    return <DocsPage theme={theme} toggle={toggle} go={go} slug={docSlug} />
  if (page === "studio") return <Studio preview />
  if (page === "packages") return <Packages />
  return <Landing theme={theme} toggle={toggle} go={go} />
}
