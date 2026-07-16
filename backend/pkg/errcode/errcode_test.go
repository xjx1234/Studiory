package errcode

import "testing"

func TestWithMessage_DoesNotMutateOriginal(t *testing.T) {
	original := ErrValidation
	originalMsgID := original.MsgID

	derived := original.WithMessage("custom_msg_id")

	if original.MsgID != originalMsgID {
		t.Fatalf("WithMessage mutated the original error: MsgID = %q, want %q", original.MsgID, originalMsgID)
	}
	if derived.MsgID != "custom_msg_id" {
		t.Errorf("derived.MsgID = %q, want %q", derived.MsgID, "custom_msg_id")
	}
	if derived.Code != original.Code {
		t.Errorf("derived.Code = %d, want %d (should be preserved)", derived.Code, original.Code)
	}
	if derived.HTTPStatus != original.HTTPStatus {
		t.Errorf("derived.HTTPStatus = %d, want %d (should be preserved)", derived.HTTPStatus, original.HTTPStatus)
	}
	if derived == original {
		t.Error("WithMessage should return a new *Error, not the same pointer")
	}
}

// allDefinedErrors 汇总当前包内定义的所有错误码，供下方一致性检查复用。
// 新增错误码时请同步加进这里——这正是这组测试的意义：防止拷贝改码时漏改导致重复。
func allDefinedErrors() map[string]*Error {
	return map[string]*Error{
		"OK":                    OK,
		"ErrUnauthorized":       ErrUnauthorized,
		"ErrInvalidToken":       ErrInvalidToken,
		"ErrTokenExpired":       ErrTokenExpired,
		"ErrInvalidCredentials": ErrInvalidCredentials,
		"ErrWrongPassword":      ErrWrongPassword,
		"ErrSamePassword":       ErrSamePassword,
		"ErrAccountLocked":      ErrAccountLocked,
		"ErrAccountDisabled":    ErrAccountDisabled,
		"ErrCannotModifySelf":   ErrCannotModifySelf,
		"ErrInvalidCode":        ErrInvalidCode,
		"ErrUnsupportedGrant":   ErrUnsupportedGrant,
		"ErrForbidden":          ErrForbidden,
		"ErrBadRequest":         ErrBadRequest,
		"ErrValidation":         ErrValidation,
		"ErrTooManyRequests":    ErrTooManyRequests,
		"ErrNotFound":           ErrNotFound,
		"ErrAlreadyExists":      ErrAlreadyExists,
		"ErrInternal":           ErrInternal,
		"ErrServiceUnavailable": ErrServiceUnavailable,
	}
}

func TestErrorCodes_AreUnique(t *testing.T) {
	seen := make(map[int]string)
	for name, e := range allDefinedErrors() {
		if existing, ok := seen[e.Code]; ok {
			t.Errorf("duplicate error code %d: %s and %s", e.Code, existing, name)
			continue
		}
		seen[e.Code] = name
	}
}

func TestErrorCodes_HaveNonEmptyMsgID(t *testing.T) {
	for name, e := range allDefinedErrors() {
		if e.MsgID == "" {
			t.Errorf("%s has empty MsgID", name)
		}
	}
}

func TestErrorCodes_HaveValidHTTPStatus(t *testing.T) {
	for name, e := range allDefinedErrors() {
		if e.HTTPStatus < 100 || e.HTTPStatus > 599 {
			t.Errorf("%s has out-of-range HTTPStatus = %d", name, e.HTTPStatus)
		}
	}
}

func TestOK_HasZeroCode(t *testing.T) {
	if OK.Code != 0 {
		t.Errorf("OK.Code = %d, want 0 (0 表示成功，是响应结构里判断成功/失败的约定)", OK.Code)
	}
}
