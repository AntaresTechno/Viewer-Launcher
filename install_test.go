package main

import "testing"

func TestLegacyLibManifestDoesNotRequireBundleMetadata(t *testing.T) {
	manifest := libManifest{UpstreamCommit: "e147f9b98da5aaedde850557b3d8e55954dea4d9"}
	if manifest.hasBundleMetadata() {
		t.Fatal("legacy manifest was treated as a split bundle")
	}
	if manifest.usesSplitBundle() {
		t.Fatal("legacy manifest selected the split installer")
	}
}

func TestPartialBundleMetadataIsRejected(t *testing.T) {
	manifest := libManifest{BundleFormat: splitBundleFormat}
	if !manifest.hasBundleMetadata() {
		t.Fatal("partial bundle metadata was not detected")
	}
	if err := manifest.validateBundle(); err == nil {
		t.Fatal("partial bundle metadata was accepted")
	}
}
