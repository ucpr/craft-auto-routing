package craft

// Folder represents a Craft folder, possibly with nested children.
type Folder struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	DocumentCount int      `json:"documentCount"`
	Folders       []Folder `json:"folders"`
}

// Document represents a Craft document.
type Document struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	LastModifiedAt string `json:"lastModifiedAt,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	ClickableLink  string `json:"clickableLink,omitempty"`
}

// Block represents a node in a Craft document's content tree.
type Block struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Markdown string  `json:"markdown,omitempty"`
	Content  []Block `json:"content,omitempty"`
}

type foldersResponse struct {
	Items []Folder `json:"items"`
}

type documentsResponse struct {
	Items []Document `json:"items"`
}

type moveRequest struct {
	DocumentIDs []string        `json:"documentIds"`
	Destination moveDestination `json:"destination"`
}

type moveDestination struct {
	FolderID string `json:"folderId"`
}
