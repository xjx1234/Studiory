package service

import (
	"backend/internal/service/domainerr"
	"backend/pkg/errcode"

	"github.com/google/uuid"
)

// ParseUUID 将外部传入的字符串 ID 统一转换为 uuid.UUID。
// 仍返回 *errcode.Error，供尚未迁移到 domainerr 的模块使用；
// 新迁移的模块请用 ParseUUIDError。
func ParseUUID(id string) (uuid.UUID, *errcode.Error) {
	parsed, err := ParseUUIDError(id)
	if err != nil {
		return uuid.Nil, errcode.ErrBadRequest
	}
	return parsed, nil
}

func ParseUUIDPair(first, second string) (uuid.UUID, uuid.UUID, *errcode.Error) {
	parsedFirst, e := ParseUUID(first)
	if e != nil {
		return uuid.Nil, uuid.Nil, e
	}
	parsedSecond, e := ParseUUID(second)
	if e != nil {
		return uuid.Nil, uuid.Nil, e
	}
	return parsedFirst, parsedSecond, nil
}

// ParseUUIDError 将外部传入的字符串 ID 转换为 uuid.UUID。
// 非法 UUID 统一视为 invalid_argument，由 HTTP 适配层映射为 400，
// 避免各业务模块重复判断。
func ParseUUIDError(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, domainerr.Wrap(domainerr.CodeInvalidArgument, "invalid uuid", err)
	}
	return parsed, nil
}

// ParseUUIDPairError 是 ParseUUIDPair 的 domainerr 版本。
func ParseUUIDPairError(first, second string) (uuid.UUID, uuid.UUID, error) {
	parsedFirst, err := ParseUUIDError(first)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	parsedSecond, err := ParseUUIDError(second)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return parsedFirst, parsedSecond, nil
}

// CanonicalUserID 把用户 ID 归一化成 canonical UUID 字符串（小写 + 连字符）。
//
// 用于一切「由 user id 拼出来的存储键」（Redis 会话键、revoke:uid 键等）：
// uuid.Parse 接受大写、`{...}`、`urn:uuid:` 等形式，键名字符串却不接受。
// 吊销类操作的写入方（管理员请求的路径参数）和读取方（JWT claim 里的 uid）来自两条不同链路，
// 只要形式不一致，写入的键就永远读不到——接口照样返回成功，旧 token 却一个都没被吊销。
//
// 非 UUID 输入原样返回：既兼容历史上直接以 "user-1" 之类字符串作 ID 的测试/迁移数据，
// 也保证不会把非法输入静默改写成另一个键名。
func CanonicalUserID(id string) string {
	if uid, err := uuid.Parse(id); err == nil {
		return uid.String()
	}
	return id
}
