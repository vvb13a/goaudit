package checks

import (
	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

func All(linkCache *service.LinkCache) []domain.Check {
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
		NewTransferStatsLogCheck(),
		NewPerformanceTimingsCheck(),
		NewSchemaCheck(),

		// Injected Link Cache checks:
		NewInternalLinksCheck(linkCache),
		NewExternalLinksCheck(linkCache),
	}
}
