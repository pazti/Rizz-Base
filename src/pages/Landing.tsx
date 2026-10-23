import type { Go, PageProps } from "../types";
import { repositoryUrl } from "../types";
import { Footer, Header } from "../components/layout";
import { BigBanner, Button, ComingSoonNotice, Icon, Logo, Ticker } from "../components/ui";
import { lobbies } from "../sample-data";

function MiniDashboard({ go }: { go: Go }) {
  return (
    <div className="mini-dashboard">
      <div className="mini-side">
        <Logo />
        <span className="active"><Icon name="data" /> Data</span>
        <span><Icon name="realtime" /> Realtime</span>
        <span><Icon name="rules" /> Rules</span>
      </div>
      <div className="mini-main">
        <div className="mini-head"><div><small>LOCAL PREVIEW</small><h3>lobbies</h3></div><Button tone="pink" onClick={() => go("dashboard")}><Icon name="plus" /> Add lobby</Button></div>
        <div className="mini-tabs"><b>Table</b><span>JSON</span><span>Vibes</span></div>
        <div className="mini-table">
          <div><b>TYPE</b><b>LOBBY</b><b>STORAGE</b></div>
          {lobbies.slice(0, 3).map((lobby) => <div key={lobby.name}><span>Sample</span><strong>{lobby.name}</strong><span>Local file</span></div>)}
        </div>
        <div className="code-strip"><span>await db</span>.createYap(lobby.id, <b>"welcome"</b>, data);</div>
      </div>
    </div>
  );
}

export default function Landing({ theme, toggle, go }: PageProps) {
  return (
    <>
      <Ticker />
      <ComingSoonNotice message="Preview only: the dashboard below uses sample data and is not connected to local .rizz files. Install the engine first, then use the Go CLI, Node.js SDK, or Python SDK. See Docs → Installation." />
      <Header theme={theme} toggle={toggle} go={go} />
      <main className="landing">
        <section className="hero">
          <div className="hero-copy">
            <div className="label"><i /> FIRESTORE’S UNHINGED COUSIN</div>
            <h1>RIZZ <span>BASE</span></h1>
            <p>The database that<br />actually has <mark>aura.</mark></p>
            <div className="button-row">
              <a className="btn" href="https://www.npmjs.com/package/rizzbase" target="_blank" rel="noreferrer">INSTALL SDK <Icon name="arrow" /></a>
              <a className="btn plain" href={repositoryUrl} target="_blank" rel="noreferrer"><Icon name="github" /> VIEW ON GITHUB</a>
            </div>
          </div>
          <div className="hero-product">
            <div className="burst">✦</div>
            <div className="live-sticker">● UI PREVIEW</div>
            <MiniDashboard go={go} />
    <div className="install">LOCAL V1 <b>GO + NODE + PYTHON</b></div>
          </div>
        </section>

        <section className="loud-strip"><small>BUILT FOR DEVELOPERS WHO ARE</small><h2>TIRED OF BORING <span>DATABASES.</span></h2></section>

        <section className="feature-section">
          <div className="section-title"><span>[ THE SAUCE ]</span><h2>ALL GAS.<br />NO BOILERPLATE.</h2></div>
          <div className="feature-grid">
            {[
              ["data", "LOCAL FIRST", "One encrypted .rizz Project file lives on your device. No hosted database or network service.", "yellow"],
              ["data", "YAPS THAT FIT", "Store regular JSON documents in a Lobby, then filter them with simple local queries.", "pink"],
              ["rules", "ENCRYPTED BY DEFAULT", "AES-256-GCM protects your local Project. Keep your passphrase private and keep a backup.", "cream"],
            ].map(([icon, title, copy, tone]) => (
              <article className={`feature-card ${tone}`} key={title}>
                <div><span><Icon name={icon} /></span></div>
                <h3>{title}</h3><p>{copy}</p><button onClick={() => go("dashboard")}>SEE THE RECEIPTS <Icon name="arrow" /></button>
              </article>
            ))}
          </div>
        </section>
        <BigBanner />
      </main>
      <Footer go={go} />
    </>
  );
}
