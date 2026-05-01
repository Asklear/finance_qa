package feishusync

func DefaultSources() []SyncSource {
	return []SyncSource{
		{
			SourceType:  SourceTypeFinanceWorkbook,
			SourceToken: "Iel5bFZWSoGF7hxjyPpcn5Elnqd",
			SourceURL:   "https://ucngfmhi7qmy.feishu.cn/file/Iel5bFZWSoGF7hxjyPpcn5Elnqd",
			DisplayName: "飞书财务表格",
			SyncMode:    "active_scan",
			SyncStatus:  SyncStatusActive,
		},
		{
			SourceType:  SourceTypePDFFolder,
			SourceToken: "JeTEfS3qQly8RJd0CJNcASumnCg",
			SourceURL:   "https://ucngfmhi7qmy.feishu.cn/drive/folder/JeTEfS3qQly8RJd0CJNcASumnCg",
			DisplayName: "飞书 PDF 文件夹 1",
			SyncMode:    "active_scan",
			SyncStatus:  SyncStatusActive,
		},
		{
			SourceType:  SourceTypePDFFolder,
			SourceToken: "S4Q0fl7AwlUbjedXUzDcP0panid",
			SourceURL:   "https://ucngfmhi7qmy.feishu.cn/drive/folder/S4Q0fl7AwlUbjedXUzDcP0panid",
			DisplayName: "飞书 PDF 文件夹 2",
			SyncMode:    "active_scan",
			SyncStatus:  SyncStatusActive,
		},
		{
			SourceType:  SourceTypePDFFolder,
			SourceToken: "FB8dfZLpQlHmuFdwsWKc5tJ5nJc",
			SourceURL:   "https://ucngfmhi7qmy.feishu.cn/drive/folder/FB8dfZLpQlHmuFdwsWKc5tJ5nJc",
			DisplayName: "飞书 PDF 文件夹 3",
			SyncMode:    "active_scan",
			SyncStatus:  SyncStatusActive,
		},
	}
}
