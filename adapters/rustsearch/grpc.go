package rustsearch

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/core/searchworker"
	"github.com/navidrome/navidrome/core/searchworker/gen"
)

func (e *Engine) startGRPC() error {
	client := searchworker.SearchClient()
	if client == nil {
		return searchworker.ErrNoGRPC
	}
	e.grpc = client
	return nil
}

func (e *Engine) grpcRoundTrip(ctx context.Context, req request) (response, error) {
	if e.grpc == nil {
		return response{}, errors.New("search gRPC client closed")
	}
	return searchworker.CallSearch(ctx, func(ctx context.Context, client gen.SearchClient) (response, error) {
		return e.grpcRoundTripOnce(ctx, client, req)
	})
}

func (e *Engine) grpcRoundTripOnce(ctx context.Context, client gen.SearchClient, req request) (response, error) {
	if client == nil {
		return response{}, errors.New("search gRPC client closed")
	}
	resp, err := client.Apply(ctx, toIndexRequest(req))
	if err != nil {
		return response{}, err
	}
	if resp.GetProtocol() != 0 && resp.GetProtocol() != protocolVersion {
		return response{}, errors.New("unsupported Rust search protocol")
	}
	if !resp.GetOk() {
		msg := resp.GetError()
		if msg == "" {
			msg = "Rust search request failed"
		}
		return response{}, errors.New(msg)
	}
	out := response{
		Protocol:   int(resp.GetProtocol()),
		OK:         true,
		Indexed:    resp.GetIndexed(),
		Normalized: resp.GetNormalized(),
	}
	if out.Protocol == 0 {
		out.Protocol = protocolVersion
	}
	for _, group := range resp.GetGroups() {
		hits := make([]hit, len(group.GetHits()))
		for i, h := range group.GetHits() {
			hits[i] = hit{ID: h.GetId(), Score: h.GetScore()}
		}
		out.Groups = append(out.Groups, searchGroup{Kind: group.GetKind(), Hits: hits})
	}
	return out, nil
}

func toIndexRequest(req request) *gen.IndexRequest {
	docs := toProtoDocuments(req.Documents)
	switch req.Op {
	case "begin_replace":
		return &gen.IndexRequest{Op: &gen.IndexRequest_BeginReplace{BeginReplace: &gen.BeginReplace{}}}
	case "append":
		return &gen.IndexRequest{Op: &gen.IndexRequest_Append{Append: &gen.Append{Documents: docs}}}
	case "commit_replace":
		return &gen.IndexRequest{Op: &gen.IndexRequest_CommitReplace{CommitReplace: &gen.CommitReplace{}}}
	case "abort_replace":
		return &gen.IndexRequest{Op: &gen.IndexRequest_AbortReplace{AbortReplace: &gen.AbortReplace{}}}
	case "upsert":
		return &gen.IndexRequest{Op: &gen.IndexRequest_Upsert{Upsert: &gen.Upsert{Documents: docs}}}
	case "delete":
		return &gen.IndexRequest{Op: &gen.IndexRequest_Delete{Delete: &gen.Delete{Keys: req.Keys}}}
	case "commit":
		return &gen.IndexRequest{Op: &gen.IndexRequest_Commit{Commit: &gen.Commit{}}}
	case "search_all":
		searches := make([]*gen.SearchSpec, len(req.Searches))
		for i, spec := range req.Searches {
			searches[i] = &gen.SearchSpec{Kind: spec.Kind, Offset: int32(spec.Offset), Limit: int32(spec.Limit)}
		}
		return &gen.IndexRequest{Op: &gen.IndexRequest_SearchAll{SearchAll: &gen.SearchAll{
			Query:      req.Query,
			LibraryIds: req.LibraryIDs,
			Searches:   searches,
		}}}
	case "normalize_fts":
		return &gen.IndexRequest{Op: &gen.IndexRequest_NormalizeFts{NormalizeFts: &gen.NormalizeFts{Values: req.Values}}}
	default:
		return &gen.IndexRequest{}
	}
}

func toProtoDocuments(docs []document) []*gen.Document {
	if len(docs) == 0 {
		return nil
	}
	out := make([]*gen.Document, len(docs))
	for i, doc := range docs {
		out[i] = &gen.Document{
			Key:        doc.Key,
			Id:         doc.ID,
			Kind:       doc.Kind,
			LibraryIds: doc.LibraryIDs,
			Primary:    doc.Primary,
			Secondary:  doc.Secondary,
		}
	}
	return out
}
