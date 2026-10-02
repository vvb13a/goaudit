package checks

import (
	"github.com/vvb13a/goaudit/domain"
)

func All() []domain.Check {
	return []domain.Check{
		NewStatusCodeCheck(),
		NewCanonicalURLCheck(),
		NewDomSizeCheck(),
		NewDocumentSizeCheck(),
		NewEnforceHTTPSCheck(),
		NewH1Check(),
		NewHeadingHierarchyCheck(),
		NewMixedContentCheck(),
		NewHreflangCheck(),
		NewImageIntegrityCheck(),
		NewMetaDescriptionCheck(),
		NewOpenGraphCheck(),
		NewTwitterCardCheck(),
		NewViewportCheck(),
		NewTitleCheck(),
		NewRobotsMetaCheck(),
		NewPerformanceTimingsCheck(),
		NewSchemaCheck(),

		// Graph checks: evaluated once, after link validation.
		NewExternalLinksCheck(),
		NewDocumentLinksCheck(),
		NewAssetLinksCheck(),
		NewMediaLinksCheck(),
	}
}
