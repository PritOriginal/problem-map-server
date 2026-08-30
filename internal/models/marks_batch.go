package models

import "io"

// NewMark is one mark to create: the domain mark, its decoded photos and
// the per-item force flag. It is the input element of a batch creation,
// where every item carries its own force because the client queued the
// items independently while offline.
type NewMark struct {
	Mark   Mark
	Photos []io.Reader
	// Force skips the duplicate detection for this item only.
	Force bool
}

// BatchAddStatus is the outcome of a single item of a batch creation.
type BatchAddStatus string

const (
	// BatchStatusCreated means the mark was created; MarkID holds its id.
	BatchStatusCreated BatchAddStatus = "created"
	// BatchStatusDuplicate means active marks of the same type already
	// exist within the dedup radius; SimilarMarks lists them and the item
	// may be repeated with Force.
	BatchStatusDuplicate BatchAddStatus = "duplicate"
	// BatchStatusFailed means the item was rejected; Err says why.
	BatchStatusFailed BatchAddStatus = "failed"
	// BatchStatusReplayed means the item was not applied because its
	// idempotency key had already been used; MarkID holds the id stored
	// for that key. It is produced by the transport layer, not by the
	// usecase.
	BatchStatusReplayed BatchAddStatus = "replayed"
)

// BatchAddResult is the outcome of one item of a batch creation. Exactly
// one of MarkID, SimilarMarks or Err is meaningful, depending on Status.
type BatchAddResult struct {
	Status       BatchAddStatus
	MarkID       int64
	SimilarMarks []MarkWithDistance
	// Err is the failure of the item; it never aborts the rest of the batch.
	Err error
}
