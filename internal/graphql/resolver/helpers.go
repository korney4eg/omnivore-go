package resolver

import (
	"net/url"
	"time"

	"github.com/omnivore-app/omnivore/internal/graphql/model"
	dbmodel "github.com/omnivore-app/omnivore/internal/model"
)

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format(time.RFC3339)
	return &formatted
}

func subscriptionSortBy(sortBy model.SortBy) string {
	switch sortBy {
	case model.SortByCreatedTime:
		return "created_at"
	case model.SortBySavedAt:
		return "most_recent_item_date"
	default:
		return "updated_at"
	}
}

func subscriptionSortOrder(sortOrder model.SortOrder) string {
	switch sortOrder {
	case model.SortOrderAscending:
		return "ASC"
	default:
		return "DESC"
	}
}

func contentReaderValue(item *dbmodel.LibraryItem) model.ContentReader {
	if item.ItemType == nil {
		return model.ContentReaderWeb
	}

	switch *item.ItemType {
	case dbmodel.ContentReaderPDF:
		return model.ContentReaderPDF
	case dbmodel.ContentReaderEPUB:
		return model.ContentReaderEpub
	default:
		return model.ContentReaderWeb
	}
}

func pageTypeValue(item *dbmodel.LibraryItem) model.PageType {
	if item.ItemType == nil {
		return model.PageTypeArticle
	}

	switch *item.ItemType {
	case dbmodel.ContentReaderPDF, dbmodel.ContentReaderEPUB:
		return model.PageTypeFile
	default:
		return model.PageTypeArticle
	}
}

func siteNameValue(item *dbmodel.LibraryItem) *string {
	if item.Site != nil && *item.Site != "" {
		return item.Site
	}

	parsed, err := url.Parse(item.OriginalURL)
	if err != nil || parsed.Host == "" {
		return nil
	}

	return strPtr(parsed.Host)
}

func labelToGraphQL(label dbmodel.Label) *model.Label {
	gqlLabel := &model.Label{
		ID:    label.ID.String(),
		Name:  label.Name,
		Color: label.Color,
	}
	if label.Description != nil {
		gqlLabel.Description = label.Description
	}
	createdAt := label.CreatedAt.Format(time.RFC3339)
	gqlLabel.CreatedAt = &createdAt
	return gqlLabel
}

func highlightTypeValue(highlight dbmodel.Highlight) string {
	if highlight.HighlightType == nil || *highlight.HighlightType == "" {
		return string(dbmodel.HighlightTypeHighlight)
	}
	return string(*highlight.HighlightType)
}

func highlightToGraphQL(highlight dbmodel.Highlight) *model.Highlight {
	gqlHighlight := &model.Highlight{
		ID:          highlight.ID.String(),
		Type:        highlightTypeValue(highlight),
		ShortID:     highlight.ShortID,
		Quote:       highlight.Quote,
		Prefix:      highlight.Prefix,
		Suffix:      highlight.Suffix,
		Patch:       highlight.Patch,
		Annotation:  highlight.Annotation,
		Color:       highlight.Color,
		CreatedByMe: true,
		CreatedAt:   highlight.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   highlight.UpdatedAt.Format(time.RFC3339),
		Labels:      make([]*model.Label, 0, len(highlight.Labels)),
	}
	if highlight.SharedAt != nil {
		sharedAt := highlight.SharedAt.Format(time.RFC3339)
		gqlHighlight.SharedAt = &sharedAt
	}
	if highlight.HighlightPositionPercent != nil {
		gqlHighlight.HighlightPositionPercent = highlight.HighlightPositionPercent
	}
	if highlight.HighlightPositionAnchorIndex != nil {
		anchorIndex := int32(*highlight.HighlightPositionAnchorIndex)
		gqlHighlight.HighlightPositionAnchorIndex = &anchorIndex
	}
	for _, label := range highlight.Labels {
		gqlHighlight.Labels = append(gqlHighlight.Labels, labelToGraphQL(label))
	}
	return gqlHighlight
}

func libraryItemToGraphQL(item *dbmodel.LibraryItem) *model.LibraryItem {
	gqlItem := &model.LibraryItem{
		ID:                         item.ID.String(),
		PageID:                     strPtr(item.ID.String()),
		Slug:                       item.Slug,
		URL:                        item.OriginalURL,
		PageType:                   pageTypeValue(item),
		ContentReader:              contentReaderValue(item),
		CreatedAt:                  item.CreatedAt.Format(time.RFC3339),
		State:                      model.LibraryItemState(item.State),
		SavedAt:                    item.SavedAt.Format(time.RFC3339),
		ReadingProgressPercent:     item.ReadingProgressPercent,
		ReadingProgressTopPercent:  &item.ReadingProgressPercent,
		ReadingProgressAnchorIndex: int32(item.ReadingProgressAnchorIndex),
		OwnedByViewer:              boolPtr(true),
		OriginalArticleURL:         strPtr(item.OriginalURL),
		SiteName:                   siteNameValue(item),
		Subscription:               item.Subscription,
		Folder:                     item.Folder,
		Labels:                     make([]*model.Label, 0, len(item.Labels)),
		Highlights:                 make([]*model.Highlight, 0, len(item.Highlights)),
	}

	if item.Title != nil {
		gqlItem.Title = item.Title
	}
	if item.Author != nil {
		gqlItem.Author = item.Author
	}
	if item.Description != nil {
		gqlItem.Description = item.Description
	}
	if item.Thumbnail != nil {
		gqlItem.Thumbnail = item.Thumbnail
		gqlItem.Image = item.Thumbnail
	}
	gqlItem.ReadAt = formatTimePtr(item.ReadAt)
	gqlItem.ArchivedAt = formatTimePtr(item.ArchivedAt)
	gqlItem.PublishedAt = formatTimePtr(item.PublishedAt)
	gqlItem.UpdatedAt = formatTimePtr(&item.UpdatedAt)
	if item.WordCount != nil {
		wordCount := int32(*item.WordCount)
		gqlItem.WordsCount = &wordCount
	}

	for _, label := range item.Labels {
		gqlItem.Labels = append(gqlItem.Labels, labelToGraphQL(label))
	}

	for _, highlight := range item.Highlights {
		gqlItem.Highlights = append(gqlItem.Highlights, highlightToGraphQL(highlight))
	}

	if len(item.Highlights) > 0 {
		gqlItem.Quote = item.Highlights[0].Quote
		gqlItem.Annotation = item.Highlights[0].Annotation
	}
	highlightsCount := int32(len(item.Highlights))
	gqlItem.HighlightsCount = &highlightsCount

	return gqlItem
}

func articleToGraphQL(item *dbmodel.LibraryItem) *model.Article {
	content := ""
	if item.ReadableContent != nil {
		content = *item.ReadableContent
	}

	gqlArticle := &model.Article{
		ID:                         item.ID.String(),
		Slug:                       item.Slug,
		URL:                        item.OriginalURL,
		PageType:                   pageTypeValue(item),
		ContentReader:              contentReaderValue(item),
		CreatedAt:                  item.CreatedAt.Format(time.RFC3339),
		State:                      model.LibraryItemState(item.State),
		SavedAt:                    item.SavedAt.Format(time.RFC3339),
		ReadingProgressPercent:     item.ReadingProgressPercent,
		ReadingProgressTopPercent:  &item.ReadingProgressPercent,
		ReadingProgressAnchorIndex: int32(item.ReadingProgressAnchorIndex),
		OriginalArticleURL:         strPtr(item.OriginalURL),
		SiteName:                   siteNameValue(item),
		SiteIcon:                   item.SiteIcon,
		Subscription:               item.Subscription,
		Folder:                     item.Folder,
		LinkID:                     strPtr(item.ID.String()),
		Content:                    content,
		Labels:                     make([]*model.Label, 0, len(item.Labels)),
		Highlights:                 make([]*model.Highlight, 0, len(item.Highlights)),
		Recommendations:            make([]*model.Recommendation, 0),
	}

	if item.Title != nil {
		gqlArticle.Title = item.Title
	}
	if item.Author != nil {
		gqlArticle.Author = item.Author
	}
	if item.Description != nil {
		gqlArticle.Description = item.Description
	}
	if item.Thumbnail != nil {
		gqlArticle.Image = item.Thumbnail
	}
	gqlArticle.ReadAt = formatTimePtr(item.ReadAt)
	gqlArticle.ArchivedAt = formatTimePtr(item.ArchivedAt)
	gqlArticle.PublishedAt = formatTimePtr(item.PublishedAt)
	gqlArticle.UpdatedAt = formatTimePtr(&item.UpdatedAt)
	if item.WordCount != nil {
		wordCount := int32(*item.WordCount)
		gqlArticle.WordsCount = &wordCount
	}
	if item.UploadFileID != nil {
		uploadFileID := item.UploadFileID.String()
		gqlArticle.UploadFileID = &uploadFileID
	}

	for _, label := range item.Labels {
		gqlArticle.Labels = append(gqlArticle.Labels, labelToGraphQL(label))
	}
	for _, highlight := range item.Highlights {
		gqlArticle.Highlights = append(gqlArticle.Highlights, highlightToGraphQL(highlight))
	}

	return gqlArticle
}
