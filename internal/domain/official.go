package domain

import "time"

const OfficialManifestVersion = 1

// OfficialTarget identifies the bounded Review target whose exact accepted
// revisions were explicitly promoted. It is pointer metadata only; chapter
// content and Review evidence remain owned by their existing canonical stores.
type OfficialTarget struct {
	Scope          string `json:"scope"`
	Chapter        int    `json:"chapter,omitempty"`
	Volume         int    `json:"volume,omitempty"`
	Arc            int    `json:"arc,omitempty"`
	ThroughChapter int    `json:"through_chapter,omitempty"`
}

// OfficialRevisionRef pins one canonical ChapterRecord revision without
// duplicating chapter text or facts.
type OfficialRevisionRef struct {
	Chapter       int    `json:"chapter"`
	Revision      int    `json:"revision"`
	ContentSHA256 string `json:"content_sha256"`
}

// OfficialSelection is the durable explicit promotion decision for one Review
// target. The Review fingerprint proves which fresh quality evidence authorized
// the transition.
type OfficialSelection struct {
	Target            OfficialTarget        `json:"target"`
	Revisions         []OfficialRevisionRef `json:"revisions"`
	ReviewFingerprint string                `json:"review_fingerprint"`
	PromotedAt        time.Time             `json:"promoted_at"`
}

// OfficialManifest is additive revision metadata, not a second chapter store.
type OfficialManifest struct {
	Version    int                          `json:"version"`
	Selections map[string]OfficialSelection `json:"selections"`
}
