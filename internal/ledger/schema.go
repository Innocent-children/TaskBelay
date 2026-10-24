package ledger

// schema 是账本的全部记录，对应 docs/requirements-ledger.md 的“记录和字段”。
// 时间都是 Unix 毫秒；工作区状态都是 git tree 哈希。
const schema = `
CREATE TABLE IF NOT EXISTS workspaces (
	id   INTEGER PRIMARY KEY,
	path TEXT NOT NULL UNIQUE -- git 工作区根目录的绝对路径
);

CREATE TABLE IF NOT EXISTS messages (
	id           INTEGER PRIMARY KEY, -- 显示为 m<id>
	workspace_id INTEGER NOT NULL REFERENCES workspaces(id),
	agent        TEXT NOT NULL,       -- claude、codex
	session_id   TEXT NOT NULL,
	text         TEXT NOT NULL,       -- 用户消息原文
	state        TEXT NOT NULL,       -- 到达时的工作区状态
	created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_by_workspace ON messages(workspace_id, created_at);

CREATE TABLE IF NOT EXISTS tasks (
	id                   INTEGER PRIMARY KEY, -- 显示为 t<id>
	workspace_id         INTEGER NOT NULL REFERENCES workspaces(id),
	title                TEXT NOT NULL,
	status               TEXT NOT NULL,       -- in_progress、completed、aborted
	workflow             TEXT NOT NULL,       -- auto、semi
	start_message_id     INTEGER REFERENCES messages(id) ON DELETE SET NULL,
	start_snapshot       TEXT NOT NULL,
	requirements_version INTEGER NOT NULL DEFAULT 0,
	created_at           INTEGER NOT NULL,
	updated_at           INTEGER NOT NULL,
	ended_at             INTEGER
);
CREATE INDEX IF NOT EXISTS tasks_by_workspace ON tasks(workspace_id, status);

CREATE TABLE IF NOT EXISTS requirements (
	task_id            INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	number             INTEGER NOT NULL,        -- 显示为 R<number>
	content            TEXT NOT NULL,
	quote              TEXT NOT NULL DEFAULT '',
	quote_message_id   INTEGER REFERENCES messages(id) ON DELETE SET NULL,
	quote_plan_version INTEGER,                 -- 引用来自已确认方案时的版本
	kind               TEXT NOT NULL,           -- quoted（原话需求）、inferred（推断需求）
	acceptance         TEXT NOT NULL DEFAULT '',
	status             TEXT NOT NULL,           -- todo、in_progress、done
	updated_by_agent   TEXT NOT NULL,
	updated_by_session TEXT NOT NULL,
	updated_at         INTEGER NOT NULL,
	PRIMARY KEY (task_id, number)
);

CREATE TABLE IF NOT EXISTS positions (
	task_id            INTEGER NOT NULL,
	requirement_number INTEGER NOT NULL,
	ord                INTEGER NOT NULL,        -- 在实现位置列表里的顺序
	at                 TEXT NOT NULL,           -- 位置的写法，如 flag:--format
	quote              TEXT NOT NULL DEFAULT '',
	quote_message_id   INTEGER REFERENCES messages(id) ON DELETE SET NULL,
	quote_plan_version INTEGER,
	PRIMARY KEY (task_id, requirement_number, ord),
	FOREIGN KEY (task_id, requirement_number) REFERENCES requirements(task_id, number) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS plans (
	task_id              INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	version              INTEGER NOT NULL,      -- 显示为 v<version>
	path                 TEXT NOT NULL,         -- 方案文件的位置
	files                TEXT NOT NULL,         -- 涉及的文件，JSON
	entries              TEXT NOT NULL,         -- 新增的对外入口，JSON
	deps                 TEXT NOT NULL,         -- 新增的依赖，JSON
	submitted_at         INTEGER NOT NULL,
	submitted_state      TEXT NOT NULL,
	submitted_by_agent   TEXT NOT NULL,
	submitted_by_session TEXT NOT NULL,
	status               TEXT NOT NULL,         -- pending、confirmed、superseded
	confirm_quote        TEXT NOT NULL DEFAULT '',
	confirm_message_id   INTEGER REFERENCES messages(id) ON DELETE SET NULL,
	confirmed_at         INTEGER,
	PRIMARY KEY (task_id, version)
);

CREATE TABLE IF NOT EXISTS notes (
	id         INTEGER PRIMARY KEY,
	task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	kind       TEXT NOT NULL,                   -- decision、next_step
	text       TEXT NOT NULL,
	agent      TEXT NOT NULL,
	session_id TEXT NOT NULL,
	state      TEXT NOT NULL,                   -- 写下时的工作区状态
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS checks (
	task_id              INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	state                TEXT NOT NULL,
	requirements_version INTEGER NOT NULL,
	conclusion           TEXT NOT NULL,         -- pass、hint、block
	hits                 TEXT NOT NULL,         -- 命中，JSON
	releases             TEXT NOT NULL,         -- 放行记录，JSON
	incomplete           TEXT NOT NULL,         -- 未完成的部分，JSON
	blocked_sessions     TEXT NOT NULL,         -- 拦截过的 session，JSON
	created_at           INTEGER NOT NULL,
	PRIMARY KEY (task_id, state, requirements_version)
);

CREATE TABLE IF NOT EXISTS sessions (
	agent                 TEXT NOT NULL,
	session_id            TEXT NOT NULL,
	workspace_id          INTEGER NOT NULL REFERENCES workspaces(id),
	started_at            INTEGER NOT NULL,
	last_active_at        INTEGER NOT NULL,
	last_summary_at       INTEGER,
	last_other_command_at INTEGER,
	PRIMARY KEY (agent, session_id)
);

CREATE TABLE IF NOT EXISTS verifications (
	id          INTEGER PRIMARY KEY,
	agent       TEXT NOT NULL,
	session_id  TEXT NOT NULL,
	turn_id     TEXT NOT NULL,
	tool_use_id TEXT NOT NULL,
	command     TEXT NOT NULL,                  -- 规整后的完整命令
	has_other   INTEGER NOT NULL,               -- 是否含其他命令
	state       TEXT NOT NULL,                  -- 命令开始运行时的工作区状态
	started_at  INTEGER NOT NULL,
	result      TEXT NOT NULL DEFAULT 'unknown', -- pass、fail、unknown
	exit_code   INTEGER,
	FOREIGN KEY (agent, session_id) REFERENCES sessions(agent, session_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS verifications_by_session ON verifications(agent, session_id, started_at);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
