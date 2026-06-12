package service

import (
	"backend/pkg/errcode"

	"github.com/google/uuid"
)

// ParseUUID 将外部传入的字符串 ID 统一转换为 uuid.UUID。
// service 层统一把非法 UUID 视为 BadRequest，避免各业务模块重复处理。
func ParseUUID(id string) (uuid.UUID, *errcode.Error) {
	parsed, err := uuid.Parse(id)
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
