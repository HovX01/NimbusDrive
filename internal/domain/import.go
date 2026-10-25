package domain

// ImportSummary reports what a database merge changed. Existing target rows
// always win; the summary only counts rows added by the import.
type ImportSummary struct {
	FoldersMerged           int      `json:"folders_merged"`
	FoldersCreated          int      `json:"folders_created"`
	FilesImported           int      `json:"files_imported"`
	FilesSkipped            int      `json:"files_skipped"`
	FilesRenamed            int      `json:"files_renamed"`
	PartsImported           int      `json:"parts_imported"`
	HashesImported          int      `json:"hashes_imported"`
	SharesImported          int      `json:"shares_imported"`
	EditProjectsImported    int      `json:"edit_projects_imported"`
	DataRowsImported        int      `json:"data_rows_imported"`
	BotGrantsImported       int      `json:"bot_grants_imported"`
	SocialConnectionsAdded  int      `json:"social_connections_imported"`
	SocialOAuthConfigsAdded int      `json:"social_oauth_configs_imported"`
	NodesSkipped            int      `json:"nodes_skipped"`
	Warnings                []string `json:"warnings,omitempty"`
}
