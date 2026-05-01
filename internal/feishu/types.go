package feishu

import (
	"context"
	"fmt"
)

type DriveFile struct {
	Token        string `json:"token"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	MimeType     string `json:"mime_type"`
	ParentToken  string `json:"parent_token"`
	Size         int64  `json:"size"`
	ModifiedTime string `json:"modified_time"`
	Revision     string `json:"revision"`
	DownloadURL  string `json:"download_url"`
}

type Client interface {
	ListFolderFiles(ctx context.Context, folderToken string) ([]DriveFile, error)
	GetFileMetadata(ctx context.Context, fileToken string) (DriveFile, error)
	DownloadFile(ctx context.Context, fileToken, destPath string) error
	ExportToXLSX(ctx context.Context, fileToken, destPath string) error
}

type APIError struct {
	Code int
	Msg  string
}

func (e APIError) Error() string {
	return fmt.Sprintf("feishu api error code=%d msg=%s", e.Code, e.Msg)
}
