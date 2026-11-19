export const lobbies = [
  { name: "hot-girl-deploy" },
  { name: "no-bugs-allowed" },
  { name: "ship-it-bestie" },
  { name: "main-character-api" },
  { name: "delulu-is-solulu" },
  { name: "touch-grass-prod" },
]

export const yapRows = [
  ["welcome", "mika", "just shipped auth. we are so back"],
  ["release-note", "jules", "prod is giving main character energy"],
  ["status", "kai", "zero bugs (do not fact check this)"],
  ["schema", "sasha", "the schema is lowkey eating"],
]

export const eventRows = [
  {
    id: "evt_01K6FX9",
    time: "23:42:18",
    kind: "user.signup",
    source: "web",
    data: '{"plan":"free","country":"us"}',
  },
  {
    id: "evt_01K6FX8",
    time: "23:41:52",
    kind: "deploy.completed",
    source: "cli",
    data: '{"environment":"prod","duration":184}',
  },
  {
    id: "evt_01K6FX7",
    time: "23:41:03",
    kind: "project.opened",
    source: "studio",
    data: '{"project_id":"rb_demo","version":1}',
  },
  {
    id: "evt_01K6FX6",
    time: "23:40:44",
    kind: "query.executed",
    source: "node",
    data: '{"collection":"lobbies","rows":6}',
  },
]
