package model

type FetchRequest struct {
	URLs       []string `json:"urls"`
	GraphQLURL string   `json:"graphql_url"`
	Token      string   `json:"token"`
}

type GetAccessTokenRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	EndpointURL string `json:"endpointUrl"`
}

type ConvertRequest struct {
	Markdown           string       `json:"markdown"`
	EditorSessionID    string       `json:"editor_session_id,omitempty"`
	Images             []ImageAsset `json:"images,omitempty"`
	Template           string       `json:"template"`
	Author             string       `json:"author"`
	Date               string       `json:"date"`
	Title              string       `json:"title"`
	DocumentID         string       `json:"documentId"`
	FooterText         string       `json:"footerText"`
	LineNumbersEnabled bool         `json:"lineNumbersEnabled"`
	ImageBaseURL       string       `json:"imageBaseUrl,omitempty"`
	ImageAuthToken     string       `json:"imageAuthToken,omitempty"`
}

// ImageAsset is an image supplied by the request caller. Content uses Go's
// standard JSON representation for []byte: a base64-encoded string.
// Path must be the image source path used in the Markdown.
type ImageAsset struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

type ConvertResponse struct {
	Latex     string `json:"latex"`
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
}

type GeneratePDFRequest struct {
	LatexCode      string `json:"latex_code"`
	Title          string `json:"title"`
	AssetSessionID string `json:"asset_session_id,omitempty"`
}

type StoreResponse struct {
	SessionID string `json:"session_id"`
}

// EditorSessionRequest contains the document and conversion settings to preload
// in an editor session.
type EditorSessionRequest struct {
	Markdown           string       `json:"markdown"`
	Images             []ImageAsset `json:"images,omitempty"`
	Template           string       `json:"template"`
	Author             string       `json:"author"`
	Date               string       `json:"date"`
	Title              string       `json:"title"`
	DocumentID         string       `json:"documentId"`
	FooterText         string       `json:"footerText"`
	LineNumbersEnabled bool         `json:"lineNumbersEnabled"`
	ImageBaseURL       string       `json:"imageBaseUrl,omitempty"`
	ImageAuthToken     string       `json:"imageAuthToken,omitempty"`
}

type EditorSessionResponse struct {
	SessionID string `json:"session_id"`
	EditURL   string `json:"edit_url"`
}

type EditorSession struct {
	Page     EditorSessionPage     `json:"page"`
	Settings EditorSessionSettings `json:"settings"`
}

// ImageSource is retained in a separate private Redis record and is never
// returned to a browser or embedded into the conversion output.
type ImageSource struct {
	BaseURL   string       `json:"baseUrl"`
	AuthToken string       `json:"authToken"`
	Images    []ImageAsset `json:"images,omitempty"`
}

type EditorSessionPage struct {
	Content    string `json:"content"`
	Title      string `json:"title"`
	AuthorName string `json:"authorName"`
}

type EditorSessionSettings struct {
	Template           string   `json:"template"`
	Date               string   `json:"date"`
	DocumentID         string   `json:"documentId"`
	FooterText         string   `json:"footerText"`
	LineNumbersEnabled bool     `json:"lineNumbersEnabled"`
	ImageBaseURL       string   `json:"imageBaseUrl,omitempty"`
	ImageTokenSaved    bool     `json:"imageTokenSaved"`
	ImagePaths         []string `json:"imagePaths,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type WikiPage struct {
	Path   string
	Locale string
}

type WikiContent struct {
	Path       string `json:"path,omitempty"`
	Title      string `json:"title,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
	AuthorName string `json:"authorName,omitempty"`
	Content    string `json:"content,omitempty"`
	Error      string `json:"error,omitempty"`
}
