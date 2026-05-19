//go:build !windows

package rime

import (
	"context"
	"errors"
)

func promptAppearanceSkinFile(ctx context.Context) (string, error) {
	return "", errors.New("当前平台暂不支持导入皮肤")
}
