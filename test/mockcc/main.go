// Command mockcc is a minimal in-memory mock of the Confluent Cloud IAM v2 API,
// used only for end-to-end testing of the confluent-identity-operator. It
// implements just the identity-pool and role-binding endpoints the operator
// calls, plus a /_debug/state endpoint the E2E asserts against.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

type identityPool struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description,omitempty"`
	IdentityClaim string `json:"identity_claim"`
	Filter        string `json:"filter"`
}

type roleBinding struct {
	ID         string `json:"id"`
	Principal  string `json:"principal"`
	RoleName   string `json:"role_name"`
	CRNPattern string `json:"crn_pattern"`
}

type store struct {
	mu       sync.Mutex
	pools    map[string]*identityPool
	bindings map[string]*roleBinding
	poolSeq  int64
	rbSeq    int64
}

func newStore() *store {
	return &store{pools: map[string]*identityPool{}, bindings: map[string]*roleBinding{}}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *store) handlePools(w http.ResponseWriter, r *http.Request) {
	// Path: /iam/v2/identity-providers/{provider}/identity-pools[/{id}]
	rest := strings.TrimPrefix(r.URL.Path, "/iam/v2/identity-providers/")
	parts := strings.SplitN(rest, "/", 3)
	// parts[0]=provider, parts[1]="identity-pools", parts[2]=id (optional)
	var poolID string
	if len(parts) == 3 {
		poolID = parts[2]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && poolID == "":
		var in identityPool
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		in.ID = fmt.Sprintf("pool-%06d", atomic.AddInt64(&s.poolSeq, 1))
		s.pools[in.ID] = &in
		log.Printf("CREATE pool %s display=%q filter=%q", in.ID, in.DisplayName, in.Filter)
		writeJSON(w, 201, in)
	case r.Method == http.MethodGet && poolID == "":
		data := make([]*identityPool, 0, len(s.pools))
		for _, p := range s.pools {
			data = append(data, p)
		}
		writeJSON(w, 200, map[string]any{"data": data, "metadata": map[string]string{}})
	case r.Method == http.MethodGet && poolID != "":
		p, ok := s.pools[poolID]
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, 200, p)
	case r.Method == http.MethodPatch && poolID != "":
		p, ok := s.pools[poolID]
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "not found"})
			return
		}
		var in identityPool
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if in.Filter != "" {
			p.Filter = in.Filter
		}
		if in.Description != "" {
			p.Description = in.Description
		}
		log.Printf("PATCH pool %s filter=%q", poolID, p.Filter)
		writeJSON(w, 200, p)
	case r.Method == http.MethodDelete && poolID != "":
		if _, ok := s.pools[poolID]; !ok {
			writeJSON(w, 404, map[string]string{"error": "not found"})
			return
		}
		delete(s.pools, poolID)
		log.Printf("DELETE pool %s", poolID)
		w.WriteHeader(204)
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (s *store) handleRoleBindings(w http.ResponseWriter, r *http.Request) {
	// Path: /iam/v2/role-bindings[/{id}]
	id := strings.TrimPrefix(r.URL.Path, "/iam/v2/role-bindings")
	id = strings.TrimPrefix(id, "/")

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && id == "":
		var in roleBinding
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		in.ID = fmt.Sprintf("rb-%06d", atomic.AddInt64(&s.rbSeq, 1))
		s.bindings[in.ID] = &in
		log.Printf("CREATE role-binding %s principal=%s role=%s crn=%s", in.ID, in.Principal, in.RoleName, in.CRNPattern)
		writeJSON(w, 201, in)
	case r.Method == http.MethodGet && id == "":
		principal := r.URL.Query().Get("principal")
		data := make([]*roleBinding, 0)
		for _, b := range s.bindings {
			if principal == "" || b.Principal == principal {
				data = append(data, b)
			}
		}
		writeJSON(w, 200, map[string]any{"data": data, "metadata": map[string]string{}})
	case r.Method == http.MethodDelete && id != "":
		if _, ok := s.bindings[id]; !ok {
			writeJSON(w, 404, map[string]string{"error": "not found"})
			return
		}
		delete(s.bindings, id)
		log.Printf("DELETE role-binding %s", id)
		w.WriteHeader(204)
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (s *store) handleDebug(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pools := make([]*identityPool, 0, len(s.pools))
	for _, p := range s.pools {
		pools = append(pools, p)
	}
	bindings := make([]*roleBinding, 0, len(s.bindings))
	for _, b := range s.bindings {
		bindings = append(bindings, b)
	}
	writeJSON(w, 200, map[string]any{"pools": pools, "roleBindings": bindings})
}

func main() {
	s := newStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/iam/v2/identity-providers/", s.handlePools)
	mux.HandleFunc("/iam/v2/role-bindings", s.handleRoleBindings)
	mux.HandleFunc("/iam/v2/role-bindings/", s.handleRoleBindings)
	mux.HandleFunc("/_debug/state", s.handleDebug)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

	log.Println("mockcc listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
