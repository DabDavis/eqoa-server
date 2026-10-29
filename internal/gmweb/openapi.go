package gmweb

import (
	"net/http"
	"strings"
)

// The operations as an OpenAPI 3.1 document, for a custom GPT's actions or any
// client that takes OpenAPI: each operation is POST /api/op/<name> with its
// arguments as the JSON body, called with a GM's API token as a Bearer token.
// ?core=1 gives the shorter list (the Core operations), for a client with
// room for fewer.

// openAPIDescMax is how long an operation's description may be: ChatGPT's
// actions refuse longer ones.
const openAPIDescMax = 300

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	base := s.publicBase(r)
	core := r.URL.Query().Get("core") == "1"
	paths := map[string]any{}
	for _, op := range s.opList {
		if core && !op.Core {
			continue
		}
		desc := op.Description
		if op.Scope == scopeAdmin {
			desc = "(admin) " + desc
		}
		if len(desc) > openAPIDescMax {
			room := openAPIDescMax - len("…")
			cut := strings.LastIndex(desc[:room], " ")
			if cut < 100 {
				cut = room
			}
			desc = desc[:cut] + "…"
		}
		paths["/api/op/"+op.Name] = map[string]any{
			"post": map[string]any{
				"operationId": op.Name,
				"summary":     op.Title,
				"description": desc,
				"requestBody": map[string]any{
					"required": true,
					"content":  map[string]any{"application/json": map[string]any{"schema": op.Input}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "the result",
						"content": map[string]any{"application/json": map[string]any{
							"schema": map[string]any{"type": "object", "additionalProperties": true},
						}},
					},
					"default": map[string]any{
						"description": "a refusal: {\"error\": why}",
						"content": map[string]any{"application/json": map[string]any{
							"schema": map[string]any{"type": "object", "properties": map[string]any{"error": map[string]any{"type": "string"}}},
						}},
					},
				},
			},
		}
	}
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "EQOA GM API",
			"version": s.build,
			"description": "Game-master tools for an EverQuest Online Adventures private server: its accounts, " +
				"characters, NPCs, items and spells, and the running world. Call guide first. Every change is audited " +
				"under the GM whose token is used and can be reverted (revert_change).",
		},
		"servers": []any{map[string]any{"url": base}},
		"paths":   paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"gmToken": map[string]any{"type": "http", "scheme": "bearer",
					"description": "A GM's API token, made on the panel's AI access page"},
			},
		},
		"security": []any{map[string]any{"gmToken": []any{}}},
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, doc)
}

// handleGuide is the guide as text, for a person or an agent that reads URLs.
func (s *Server) handleGuide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(s.guideFor(r)))
}
