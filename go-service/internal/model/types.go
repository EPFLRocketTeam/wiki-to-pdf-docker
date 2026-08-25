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
	Markdown           string `json:"markdown"`
	Template           string `json:"template"`
	Author             string `json:"author"`
	Date               string `json:"date"`
	Title              string `json:"title"`
	DocumentID         string `json:"documentId"`
	FooterText         string `json:"footerText"`
	LineNumbersEnabled bool   `json:"lineNumbersEnabled"`
}

type ConvertResponse struct {
	Latex     string `json:"latex"`
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
}

type GeneratePDFRequest struct {
	LatexCode string `json:"latex_code"`
	Title     string `json:"title"`
}

type StoreResponse struct {
	SessionID string `json:"session_id"`
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
