import { useState, type ReactNode } from "react";
import type { PageProps } from "../types"
import { AppShell } from "../components/layout"
import { Button, Icon } from "../components/ui"
import { eventRows, lobbies, yapRows } from "../sample-data"

export function Events({ theme, toggle, go }: PageProps) {
  const [events, setEvents] = useState(eventRows);
  const [filter, setFilter] = useState("");

  const appendEvent = () => {
    setEvents((current) => [
      {
        id: `evt_${Math.random().toString(36).slice(2, 10).toUpperCase()}`,
        time: new Date().toLocaleTimeString("en-US", { hour12: false }),
        kind: "ui.event.appended",
        source: "preview",
        data: '{"project_id":"rb_demo","version":1}',
      },
      ...current,
    ]);
  };

  const exportEvents = () => {
    const blob = new Blob([JSON.stringify(events, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "rizzbase-events.json";
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const visibleEvents = events.filter((event) =>
    `${event.kind} ${event.source} ${event.id}`.toLowerCase().includes(filter.toLowerCase()),
  );

  return (
    <AppShell page="events" theme={theme} toggle={toggle} go={go}>
      <main className="product-page events-page">
        <div className="product-heading">
          <div>
            <span className="kicker">APPEND-ONLY / SAMPLE DATA</span>
            <h1>
              EVENTS<span>.</span>
            </h1>
            <p>
              Ordered activity for analytics, logs, and local-first workflows.
            </p>
          </div>
          <Button tone="primary" onClick={appendEvent}>
            <Icon name="plus" /> APPEND EVENT
          </Button>
        </div>
        <div className="events-summary">
          <div>
            <span>TODAY</span>
            <b>1,284</b>
            <small>events captured</small>
          </div>
          <div>
            <span>PROJECT</span>
            <b>rb_demo</b>
            <small>project_id</small>
          </div>
          <div>
            <span>FORMAT</span>
            <b>v1</b>
            <small>project version</small>
          </div>
          <div>
            <span>SYNC</span>
            <b>LOCAL</b>
            <small>offline-first</small>
          </div>
        </div>
        <div className="collection-tools">
          <label className="search">
            ⌕ <input value={filter} onChange={(event) => setFilter(event.target.value)} placeholder="Filter event kind..." />
          </label>
          <div>
            <Button tone="plain">LAST 24 HOURS</Button>
            <Button tone="dark" onClick={exportEvents}>↗ EXPORT JSON</Button>
          </div>
        </div>
        <section className="events-stream">
          <div className="events-head">
            <span>TIME</span>
            <span>EVENT</span>
            <span>SOURCE</span>
            <span>PAYLOAD</span>
          </div>
          {visibleEvents.map((event) => (
            <article key={event.id}>
              <time>{event.time}</time>
              <div>
                <code>{event.id}</code>
                <b>{event.kind}</b>
              </div>
              <span className="event-source">{event.source}</span>
              <pre>{event.data}</pre>
            </article>
          ))}
        </section>
      </main>
    </AppShell>
  )
}

export function Dashboard({ theme, toggle, go }: PageProps) {
  return (
    <AppShell page="lobbies" theme={theme} toggle={toggle} go={go}>
      <main className="product-page">
        <div className="product-heading">
          <div>
            <span className="kicker">PREVIEW / SAMPLE DATA</span>
            <h1>
              LOBBIES<span>.</span>
            </h1>
            <p>Your chaotic little corners of the internet.</p>
          </div>
          <Button tone="primary">
            <Icon name="plus" /> NEW LOBBY
          </Button>
        </div>
        <div className="collection-tools">
          <div className="search">
            ⌕ <span>Search lobbies...</span>
          </div>
          <div>
            <Button tone="plain">FILTERS</Button>
            <Button tone="dark">▦ GRID</Button>
          </div>
        </div>
        <div className="lobby-grid">
          {lobbies.map((lobby) => (
            <article
              className="lobby-card"
              key={lobby.name}
              onClick={() => go("lobby")}
            >
              <div>
                <span className="status">SAMPLE</span>
                <button>•••</button>
              </div>
              <h2>{lobby.name}</h2>
              <p>Example Lobby for the local-first preview.</p>
              <div className="card-foot">
                <span>Not connected to a Project</span>
                <Icon name="arrow" />
              </div>
            </article>
          ))}
          <button className="add-lobby" type="button">
            <span>
              <Icon name="plus" />
            </span>
            <b>CREATE NEW LOBBY</b>
            <small>Go make something unhinged.</small>
          </button>
        </div>
      </main>
    </AppShell>
  )
}

export function Lobby({ theme, toggle, go }: PageProps) {
  return (
    <AppShell page="hot-girl-deploy" theme={theme} toggle={toggle} go={go}>
      <main className="product-page">
        <button className="back-link" onClick={() => go("dashboard")}>
          <Icon name="back" /> ALL LOBBIES
        </button>
        <div className="product-heading compact">
          <div>
            <div className="live-line">
              <span>SAMPLE</span>
            </div>
            <h1>
              HOT-GIRL-DEPLOY<span>.</span>
            </h1>
            <p>Example data for the interface preview.</p>
          </div>
          <Button tone="primary" onClick={() => go("editor")}>
            <Icon name="plus" /> ADD YAP
          </Button>
        </div>
        <div className="content-tabs">
          <button className="active">TABLE</button>
          <button>JSON</button>
          <button>VIBES</button>
        </div>
        <div className="lobby-layout">
          <div className="data-panel">
            <div className="table-tools">
              <div>
                ⌕ <span>Find a yap...</span>
              </div>
              <Button tone="plain">↕ SORT</Button>
            </div>
            <div className="brutal-table">
              <div className="tr th">
                <span>ID</span>
                <span>AUTHOR</span>
                <span>CONTENT</span>
              </div>
              {yapRows.map((row) => (
                <button
                  className="tr"
                  key={row[0]}
                  onClick={() => go("editor")}
                >
                  <code>{row[0]}</code>
                  <span>
                    <i>{row[1][0].toUpperCase()}</i>
                    {row[1]}
                  </span>
                  <b>{row[2]}</b>
                </button>
              ))}
            </div>
          </div>
          <aside className="json-float">
            <div>
              <span>SAMPLE JSON</span>
            </div>
            <pre>
              {`{
  "lobby": "hot-girl-deploy",
  "author": `}
              <em>"mika"</em>
              {`,
  "content": `}
              <b>"just shipped auth"</b>
              {`
}`}
            </pre>
            <button>COPY RAW JSON</button>
          </aside>
        </div>
      </main>
    </AppShell>
  )
}

function Field({
  label,
  children,
  wide,
}: {
  label: string
  children: ReactNode
  wide?: boolean
}) {
  return (
    <label className={wide ? "wide" : ""}>
      <span>{label}</span>
      {children}
    </label>
  )
}

export function Editor({ theme, toggle, go }: PageProps) {
  return (
    <AppShell page="welcome" theme={theme} toggle={toggle} go={go}>
      <main className="product-page editor">
        <div className="editor-head">
          <div>
            <button className="back-link" onClick={() => go("lobby")}>
              <Icon name="back" /> HOT-GIRL-DEPLOY
            </button>
            <h1>
              EDIT YAP<span>.</span>
            </h1>
            <p>welcome · Example record</p>
          </div>
          <div>
            <Button tone="plain">
              <Icon name="trash" /> DELETE
            </Button>
            <Button>
              <Icon name="check" /> SAVE CHANGES
            </Button>
          </div>
        </div>
        <div className="editor-grid">
          <section className="form-card">
            <div className="card-title">
              <span>YAP DETAILS</span>
              <b>REQUIRED-ish</b>
            </div>
            <div className="form-grid">
              <Field label="CONTENT" wide>
                <textarea defaultValue="just shipped auth. we are so back" />
              </Field>
              <Field label="TAGS" wide>
                <div className="tag-input">
                  <b>shipping ×</b>
                  <b>auth ×</b>
                  <span>Add a tag...</span>
                </div>
              </Field>
              <Field label="AUTHOR">
                <input defaultValue="mika" />
              </Field>
              <Field label="VISIBILITY">
                <select defaultValue="lobby">
                  <option value="lobby">Whole lobby</option>
                </select>
              </Field>
            </div>
          </section>
          <aside className="json-editor">
            <div className="card-title">
              <span>EXAMPLE JSON</span>
              <b>SAMPLE DATA</b>
            </div>
            <pre>
              {`{
  "id": `}
              <b>"welcome"</b>
              {`,
  "author": `}
              <b>"mika"</b>
              {`,
  "content": `}
              <b>"just shipped auth. we are so back"</b>
              {`,
  "tags": [`}
              <b>"shipping"</b>
              {`, `}
              <b>"auth"</b>
              {`]
}`}
            </pre>
            <div>
              <span>Example only. Not connected.</span>
              <button>COPY</button>
            </div>
          </aside>
        </div>
        <section className="history">
          <div className="card-title">
            <span>EXAMPLE HISTORY</span>
          </div>
          <div className="timeline">
            <div>
              <i className="hot">✦</i>
              <span>
                <b>Example Yap created</b>
              </span>
            </div>
            <div>
              <i>✓</i>
              <span>
                <b>Example content updated</b>
              </span>
            </div>
          </div>
        </section>
      </main>
    </AppShell>
  )
}
