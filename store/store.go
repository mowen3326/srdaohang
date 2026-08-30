package store

import (
	"context"

	"daohang/model"
)

// Store 定义数据访问契约，便于日后替换存储实现。
type Store interface {
	Close() error

	ListCategories(ctx context.Context) ([]model.Category, error)
	ListCategoriesPaged(ctx context.Context, keyword string, page, pageSize int) ([]model.Category, int, error)
	GetCategory(ctx context.Context, id int64) (*model.Category, error)
	CreateCategory(ctx context.Context, c *model.Category) error
	UpdateCategory(ctx context.Context, c *model.Category) error
	DeleteCategory(ctx context.Context, id int64) error

	ListLinks(ctx context.Context) ([]model.Link, error)
	ListLinksPaged(ctx context.Context, keyword string, categoryID int64, page, pageSize int) ([]model.Link, int, error)
	GetLink(ctx context.Context, id int64) (*model.Link, error)
	CreateLink(ctx context.Context, l *model.Link) error
	UpdateLink(ctx context.Context, l *model.Link) error
	DeleteLink(ctx context.Context, id int64) error
	DeleteLinks(ctx context.Context, ids []int64) error

	Count(ctx context.Context) (cats int, links int, err error)

	// 初始化状态与键值配置（管理员凭据存于此处）
	IsInitialized(ctx context.Context) (bool, error)
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	// 背景图管理
	ListBackgrounds(ctx context.Context) ([]model.Background, error)
	CreateBackground(ctx context.Context, b *model.Background) error
	SetActiveBackground(ctx context.Context, id int64) error
	ActiveBackground(ctx context.Context) (*model.Background, error)
	DeleteBackground(ctx context.Context, id int64) error

	// 登录限流：失败计数与锁定截止时间（key 为登录标识，如用户名+IP）
	GetLoginFailCount(ctx context.Context, key string) (int, error)
	SetLoginFailCount(ctx context.Context, key string, count int) error
	GetLoginLockUntil(ctx context.Context, key string) (int64, error)
	SetLoginLockUntil(ctx context.Context, key string, until int64) error
	DeleteLoginFail(ctx context.Context, key string) error

	// 登录审计日志
	AddLoginLog(ctx context.Context, log *model.LoginLog) error
	ListLoginLogsPaged(ctx context.Context, page, pageSize int) ([]model.LoginLog, int, error)
	ClearLoginLogs(ctx context.Context) error
	TrimLoginLogs(ctx context.Context, keep int) error
}
