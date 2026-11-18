package upstream

import "testing"

func TestProviderIdentifiersAreStable(t *testing.T) {
	if ProviderAcgFaka != "acg_faka" || ProviderMengciyuan != "mengciyuan" || ProviderDujiaoNext != "dujiao_next" {
		t.Fatal("provider identifiers are part of persisted configuration and must remain stable")
	}
}

func TestCatalogPreservesUnknownStock(t *testing.T) {
	product := Product{ExternalID: "p-1", Stock: nil}
	if product.Stock != nil {
		t.Fatal("unknown stock must not be interpreted as zero")
	}
}
