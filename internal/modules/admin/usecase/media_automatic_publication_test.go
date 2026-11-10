package usecase

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAutomaticChecksAllowPublicationButNeverReplaceRights(t *testing.T) {
	draft := `{"published":false,"media_kind":"music","rights_approved":true,"rights_reference":"License 123","hls_manifest_key":"hls/item/version/master.m3u8","transcript":[{"cue_id":"cue","start":0,"end":2,"timing_status":"auto_approved","editorial_review":{"decision":"auto_approved"},"translation":{"fa":"سلام","ckb":"سڵاو"},"translation_review":{"fa":"auto_approved","ckb":"auto_approved"},"exercise_review":"auto_approved"}]}`
	published := json.RawMessage(strings.Replace(draft, `"published":false`, `"published":true`, 1))
	if err := guardPublication(json.RawMessage(draft), published); err != nil {
		t.Fatalf("checked draft blocked: %v", err)
	}
	for _, blocked := range []string{
		strings.Replace(draft, `"rights_approved":true`, `"rights_approved":false`, 1),
		strings.Replace(draft, `"rights_reference":"License 123"`, `"rights_reference":""`, 1),
		strings.Replace(draft, `"decision":"auto_approved"`, `"decision":"draft"`, 1),
		strings.Replace(draft, `"fa":"auto_approved"`, `"fa":"draft"`, 1),
		strings.Replace(draft, `"exercise_review":"auto_approved"`, `"exercise_review":"draft"`, 1),
	} {
		if err := guardPublication(json.RawMessage(blocked), published); err == nil {
			t.Fatal("incomplete draft passed publication")
		}
	}
}

func TestAutomaticImportedStoryQualityKeepsRightsRequirement(t *testing.T) {
	payload := `{"review":{"rights_approved":true,"quality_review":"auto_approved","audio_transcript_match_verified":true},"translation_status":{"fa":"auto_approved","ckb":"auto_approved"},"transcript":[{"text":"Hello there","start":0,"end":2}]}`
	if !publicationAllowed(json.RawMessage(payload)) {
		t.Fatal("automated story checks rejected")
	}
	if publicationAllowed(json.RawMessage(strings.Replace(payload, `"rights_approved":true`, `"rights_approved":false`, 1))) {
		t.Fatal("automatic quality granted rights")
	}
}
