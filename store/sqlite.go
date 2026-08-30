package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 驱动，无需 CGO

	"daohang/model"
)

//go:embed schema.sql
var schemaFS embed.FS

// SQLite 是基于 modernc.org/sqlite 的存储实现。
type SQLite struct {
	db *sql.DB
}

// OpenSQLite 打开（必要时创建）数据库并执行建表迁移。
func OpenSQLite(path string) (*SQLite, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}

	// busy_timeout 防锁等待超时，WAL 提升读写并发，foreign_keys 启用级联删除
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)",
		path,
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 写操作串行，限制单连接避免 "database is locked"
	db.SetMaxOpenConns(1)

	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) migrate() error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("读取建表脚本失败: %w", err)
	}
	if _, err := s.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("执行建表失败: %w", err)
	}
	return nil
}

func (s *SQLite) Close() error { return s.db.Close() }

// ListCategories 按排序号、ID 升序返回全部分类。
func (s *SQLite) ListCategories(ctx context.Context) ([]model.Category, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, icon, sort_order FROM categories ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.SortOrder); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// ListCategoriesPaged 按排序号、ID 升序返回指定页的分类，并返回总条数。
// keyword 非空时按名称模糊匹配。
func (s *SQLite) ListCategoriesPaged(ctx context.Context, keyword string, page, pageSize int) ([]model.Category, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	like := "%" + keyword + "%"
	where := ""
	args := []any{}
	if keyword != "" {
		where = " WHERE name LIKE ?"
		args = append(args, like)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	qargs := append(args, pageSize, offset)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, icon, sort_order FROM categories`+where+` ORDER BY sort_order ASC, id ASC LIMIT ? OFFSET ?`,
		qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.SortOrder); err != nil {
			return nil, 0, err
		}
		list = append(list, c)
	}
	return list, total, rows.Err()
}

func (s *SQLite) GetCategory(ctx context.Context, id int64) (*model.Category, error) {
	var c model.Category
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, icon, sort_order FROM categories WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Icon, &c.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *SQLite) CreateCategory(ctx context.Context, c *model.Category) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO categories (name, icon, sort_order) VALUES (?, ?, ?)`,
		c.Name, c.Icon, c.SortOrder)
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (s *SQLite) UpdateCategory(ctx context.Context, c *model.Category) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, icon = ?, sort_order = ? WHERE id = ?`,
		c.Name, c.Icon, c.SortOrder, c.ID)
	return err
}

func (s *SQLite) DeleteCategory(ctx context.Context, id int64) error {
	// 依赖外键 ON DELETE CASCADE 清理其下链接
	_, err := s.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	return err
}

// ListLinks 按排序号、ID 升序返回全部链接。
func (s *SQLite) ListLinks(ctx context.Context) ([]model.Link, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, category_id, title, url, icon, description, sort_order, created_at
		 FROM links ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Link
	for rows.Next() {
		var l model.Link
		if err := rows.Scan(&l.ID, &l.CategoryID, &l.Title, &l.URL, &l.Icon,
			&l.Description, &l.SortOrder, &l.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

// ListLinksPaged 按排序号、ID 升序返回指定页的链接，并返回总条数。
// keyword 非空时按标题或网址模糊匹配；categoryID 大于 0 时仅返回该分类下的链接。
func (s *SQLite) ListLinksPaged(ctx context.Context, keyword string, categoryID int64, page, pageSize int) ([]model.Link, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	like := "%" + keyword + "%"
	conds := []string{}
	args := []any{}
	if keyword != "" {
		conds = append(conds, "(title LIKE ? OR url LIKE ?)")
		args = append(args, like, like)
	}
	if categoryID > 0 {
		conds = append(conds, "category_id = ?")
		args = append(args, categoryID)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM links`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	qargs := append(args, pageSize, offset)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, category_id, title, url, icon, description, sort_order, created_at
		 FROM links`+where+` ORDER BY sort_order ASC, id ASC LIMIT ? OFFSET ?`,
		qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []model.Link
	for rows.Next() {
		var l model.Link
		if err := rows.Scan(&l.ID, &l.CategoryID, &l.Title, &l.URL, &l.Icon,
			&l.Description, &l.SortOrder, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}

func (s *SQLite) GetLink(ctx context.Context, id int64) (*model.Link, error) {
	var l model.Link
	err := s.db.QueryRowContext(ctx,
		`SELECT id, category_id, title, url, icon, description, sort_order, created_at
		 FROM links WHERE id = ?`, id,
	).Scan(&l.ID, &l.CategoryID, &l.Title, &l.URL, &l.Icon,
		&l.Description, &l.SortOrder, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *SQLite) CreateLink(ctx context.Context, l *model.Link) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO links (category_id, title, url, icon, description, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		l.CategoryID, l.Title, l.URL, l.Icon, l.Description, l.SortOrder)
	if err != nil {
		return err
	}
	l.ID, err = res.LastInsertId()
	return err
}

func (s *SQLite) UpdateLink(ctx context.Context, l *model.Link) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE links SET category_id = ?, title = ?, url = ?, icon = ?, description = ?, sort_order = ?
		 WHERE id = ?`,
		l.CategoryID, l.Title, l.URL, l.Icon, l.Description, l.SortOrder, l.ID)
	return err
}

func (s *SQLite) DeleteLink(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM links WHERE id = ?`, id)
	return err
}

// DeleteLinks 批量删除指定 ID 的链接，无匹配项时静默成功。
func (s *SQLite) DeleteLinks(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM links WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}

// IsInitialized 判断管理员账号是否已创建。
func (s *SQLite) IsInitialized(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM settings WHERE key = 'admin_hash'`).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetSetting 读取配置值，不存在时返回空串。
func (s *SQLite) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSetting 写入配置值，已存在则覆盖。
func (s *SQLite) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

func (s *SQLite) Count(ctx context.Context) (int, int, error) {
	var cats, links int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories`).Scan(&cats); err != nil {
		return 0, 0, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM links`).Scan(&links); err != nil {
		return 0, 0, err
	}
	return cats, links, nil
}

// ListBackgrounds 按 ID 升序返回全部背景图。
func (s *SQLite) ListBackgrounds(ctx context.Context) ([]model.Background, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, filename, original, active FROM backgrounds ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Background
	for rows.Next() {
		var b model.Background
		var active int
		if err := rows.Scan(&b.ID, &b.Filename, &b.Original, &active); err != nil {
			return nil, err
		}
		b.Active = active != 0
		list = append(list, b)
	}
	return list, rows.Err()
}

// CreateBackground 新增一条背景图记录。若为第一张，则默认生效。
func (s *SQLite) CreateBackground(ctx context.Context, b *model.Background) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO backgrounds (filename, original, active)
		 VALUES (?, ?, (SELECT CASE WHEN COUNT(*) = 0 THEN 1 ELSE 0 END FROM backgrounds))`,
		b.Filename, b.Original)
	if err != nil {
		return err
	}
	b.ID, err = res.LastInsertId()
	return err
}

// SetActiveBackground 将指定背景图设为生效，其余置为未生效。
func (s *SQLite) SetActiveBackground(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE backgrounds SET active = 0`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE backgrounds SET active = 1 WHERE id = ?`, id)
	return err
}

// DeleteBackground 删除指定背景图记录。
func (s *SQLite) DeleteBackground(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backgrounds WHERE id = ?`, id)
	return err
}

// ActiveBackground 返回当前生效的背景图，未设置时返回 nil。
func (s *SQLite) ActiveBackground(ctx context.Context) (*model.Background, error) {
	var b model.Background
	var active int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, filename, original, active FROM backgrounds WHERE active = 1 LIMIT 1`,
	).Scan(&b.ID, &b.Filename, &b.Original, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.Active = active != 0
	return &b, nil
}

// loginFailKey 与 loginLockKey 生成限流状态在 settings 表中的存储键。
func loginFailKey(key string) string { return "login_fail:" + key }
func loginLockKey(key string) string { return "login_lock:" + key }

// GetLoginFailCount 读取指定登录标识的失败次数，无记录时为 0。
func (s *SQLite) GetLoginFailCount(ctx context.Context, key string) (int, error) {
	v, err := s.GetSetting(ctx, loginFailKey(key))
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

// SetLoginFailCount 写入失败次数，同时记录锁定截止时间戳（count 达上限时由调用方保证）。
func (s *SQLite) SetLoginFailCount(ctx context.Context, key string, count int) error {
	return s.SetSetting(ctx, loginFailKey(key), strconv.Itoa(count))
}

// DeleteLoginFail 清除失败计数与锁定状态（登录成功时调用）。
func (s *SQLite) DeleteLoginFail(ctx context.Context, key string) error {
	if err := s.SetSetting(ctx, loginFailKey(key), ""); err != nil {
		return err
	}
	return s.SetSetting(ctx, loginLockKey(key), "")
}

// GetLoginLockUntil 返回指定标识的锁定截止 Unix 时间戳，未锁定时为 0。
func (s *SQLite) GetLoginLockUntil(ctx context.Context, key string) (int64, error) {
	v, err := s.GetSetting(ctx, loginLockKey(key))
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

// SetLoginLockUntil 写入锁定截止 Unix 时间戳，0 表示解锁。
func (s *SQLite) SetLoginLockUntil(ctx context.Context, key string, until int64) error {
	if until <= 0 {
		return s.SetSetting(ctx, loginLockKey(key), "")
	}
	return s.SetSetting(ctx, loginLockKey(key), strconv.FormatInt(until, 10))
}

// maxLoginLogs 日志保留上限，超出后自动裁剪最旧的记录，防止无限增长。
const maxLoginLogs = 1000

// AddLoginLog 追加一条登录日志，并裁剪超出上限的旧记录。
func (s *SQLite) AddLoginLog(ctx context.Context, log *model.LoginLog) error {
	success := 0
	if log.Success {
		success = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO login_logs (username, ip, success, reason) VALUES (?, ?, ?, ?)`,
		log.Username, log.IP, success, log.Reason)
	if err != nil {
		return err
	}
	log.ID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	// 保留最近 maxLoginLogs 条，删除更早的
	return s.TrimLoginLogs(ctx, maxLoginLogs)
}

// ListLoginLogsPaged 按时间倒序返回指定页的登录日志，并返回总条数。
func (s *SQLite) ListLoginLogsPaged(ctx context.Context, page, pageSize int) ([]model.LoginLog, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM login_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, ip, success, reason, created_at
		 FROM login_logs ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list, err := scanLoginLogs(rows)
	return list, total, err
}

// ClearLoginLogs 清空全部登录日志。
func (s *SQLite) ClearLoginLogs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_logs`)
	return err
}

// TrimLoginLogs 仅保留最新 keep 条日志，删除更早的记录，防止日志无限增长。
func (s *SQLite) TrimLoginLogs(ctx context.Context, keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM login_logs
		WHERE id NOT IN (SELECT id FROM login_logs ORDER BY id DESC LIMIT ?)`, keep)
	return err
}

// scanLoginLogs 将查询结果扫描为日志列表，时间统一转为中国时区。
func scanLoginLogs(rows *sql.Rows) ([]model.LoginLog, error) {
	cst := time.FixedZone("CST", 8*3600)

	var list []model.LoginLog
	for rows.Next() {
		var l model.LoginLog
		var success int
		var raw string
		if err := rows.Scan(&l.ID, &l.Username, &l.IP, &success, &l.Reason, &raw); err != nil {
			return nil, err
		}
		l.Success = success != 0
		l.CreatedAt = formatCST(raw, cst)
		list = append(list, l)
	}
	return list, rows.Err()
}

// formatCST 将 SQLite 的 UTC 时间串（如 "2026-08-31 00:44:30"）转换为中国时间 "2006-01-02 15:04:05"。
// 解析失败时原样返回，避免因格式异常丢失信息。
func formatCST(raw string, loc *time.Location) string {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
			return t.In(loc).Format("2006-01-02 15:04:05")
		}
	}
	return raw
}
