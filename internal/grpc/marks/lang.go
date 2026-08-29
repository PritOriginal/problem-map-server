package marksgrpc

import (
	"context"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"google.golang.org/grpc/metadata"
)

// langMetadataKey is the incoming metadata key carrying the client's
// language preference, in the Accept-Language header syntax.
const langMetadataKey = "accept-language"

// langFromMetadata resolves the dictionaries' language from the incoming
// metadata; models.DefaultLang without a supported value.
func langFromMetadata(ctx context.Context) models.Lang {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return models.DefaultLang
	}
	return models.ParseAcceptLanguage(strings.Join(md.Get(langMetadataKey), ","))
}
