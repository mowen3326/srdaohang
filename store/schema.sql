CREATE TABLE IF NOT EXISTS categories (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    icon       TEXT    NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    title       TEXT    NOT NULL,
    url         TEXT    NOT NULL,
    icon        TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_links_category ON links(category_id, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_categories_sort ON categories(sort_order, id);

-- 键值配置表：存放管理员账号与密码哈希等运行期配置
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- 背景图表：存放上传的前台背景图及其生效状态
CREATE TABLE IF NOT EXISTS backgrounds (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    filename   TEXT    NOT NULL,              -- 磁盘上的存储文件名（位于 uploads 目录）
    original   TEXT    NOT NULL DEFAULT '',   -- 上传时的原始文件名
    active     INTEGER NOT NULL DEFAULT 0,    -- 1 表示当前生效
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 登录审计日志表：记录每次登录成功/失败及锁定事件
CREATE TABLE IF NOT EXISTS login_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    username   TEXT    NOT NULL DEFAULT '',   -- 尝试登录的用户名
    ip         TEXT    NOT NULL DEFAULT '',   -- 来源 IP
    success    INTEGER NOT NULL DEFAULT 0,    -- 1 表示成功，0 表示失败
    reason     TEXT    NOT NULL DEFAULT '',   -- 失败原因（如 locked / bad_credentials）
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_login_logs_created ON login_logs(created_at DESC);
