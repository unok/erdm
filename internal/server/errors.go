package server

import (
	"encoding/json"
	"net/http"
)

// errorResponse は HTTP API のエラーレスポンス本体（design.md §エラー処理）。
// `code` は snake_case で機械可読な分類、`message` は人間可読な説明、
// `detail` は省略可能な追加情報（パースエラー位置など）。
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

// errorEnvelope は `{ "error": { ... } }` 形式の包み構造。
type errorEnvelope struct {
	Error errorResponse `json:"error"`
}

// writeJSONError は detail 無しのエラー応答を書き出す。
//
// Content-Type を application/json に設定し、status コードと共に envelope を
// 返す。Encode 失敗はクライアント切断などが主因のため、戻り値で握り潰さない
// （ヘッダ送出済みのため http.Error で再書き込み不可）。
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSONErrorWithDetail(w, status, code, message, nil)
}

// writeJSONErrorWithDetail は detail 付きエラー応答を書き出す。
//
// detail が nil の場合は omitempty により JSON に含まれない。
func writeJSONErrorWithDetail(w http.ResponseWriter, status int, code, message string, detail any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	envelope := errorEnvelope{Error: errorResponse{Code: code, Message: message, Detail: detail}}
	if err := json.NewEncoder(w).Encode(envelope); err != nil {
		return
	}
}
