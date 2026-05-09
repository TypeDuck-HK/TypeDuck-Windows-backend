//go:build !windows

package rime

import (
	"context"
	"errors"
)

func promptSchemeSetDownloadPackage(ctx context.Context) (schemeSetDownloadPackage, error) {
	return schemeSetDownloadPackage{}, errors.New("当前平台暂不支持方案集下载窗口")
}
