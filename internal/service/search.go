package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
)

// SearchService handles article search with filtering, sorting, and pagination.
type SearchService struct {
	db *gorm.DB
}

// NewSearchService creates a new search service.
func NewSearchService(database *gorm.DB) *SearchService {
	return &SearchService{db: database}
}

// SearchParams holds search query parameters.
type SearchParams struct {
	UserID         uuid.UUID
	Query          string
	After          *string
	First          *int
	IncludeContent bool
}

// SearchResult holds search results with pagination.
type SearchResult struct {
	Items           []model.LibraryItem
	HasNextPage     bool
	HasPreviousPage bool
	StartCursor     *string
	EndCursor       *string
	TotalCount      *int
}

// Search performs article search with filters, sorting, and pagination.
func (s *SearchService) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	// Parse offset from cursor
	offset := 0
	if params.After != nil {
		fmt.Sscanf(*params.After, "%d", &offset)
	}

	// Default and max page size
	pageSize := 10
	if params.First != nil {
		pageSize = *params.First
		if pageSize > 100 {
			pageSize = 100
		}
	}

	var items []model.LibraryItem
	var totalCount int64
	filters := parseSearchQuery(params.Query)
	folderFilter := lastFilterValue(filters, "in")

	err := db.AuthTx(ctx, s.db, func(tx *gorm.DB) error {
		// Build query
		query := tx.Model(&model.LibraryItem{}).
			Preload("Labels").
			Preload("Highlights").
			Where("user_id = ?", params.UserID)

		switch folderFilter {
		case "trash":
			query = query.Where("state = ?", model.LibraryItemStateDeleted)
		case "archive":
			query = query.Where("state = ?", model.LibraryItemStateArchived)
		default:
			query = query.Where("state = ?", model.LibraryItemStateSucceeded)
		}

		if folderFilter != "" && folderFilter != "all" && folderFilter != "trash" && folderFilter != "archive" {
			query = query.Where("folder = ?", folderFilter)
		} else if params.Query == "" {
			query = query.Where("folder = ?", "inbox")
		}

		// Apply search filters
		if len(filters) > 0 {
			query = s.applySearchFilters(query, filters)
		}

		// Count total (expensive, should be cached in production)
		if err := query.Count(&totalCount).Error; err != nil {
			return fmt.Errorf("failed to count items: %w", err)
		}

		// Apply pagination
		// Fetch size+1 to determine hasNextPage
		query = query.Offset(offset).Limit(pageSize + 1)

		// Apply sorting (default: saved_at DESC)
		query = s.applySorting(query, params.Query)

		// Select columns (exclude content unless requested)
		if !params.IncludeContent {
			query = query.Omit("ReadableContent")
		}

		// Execute query
		if err := query.Find(&items).Error; err != nil {
			return fmt.Errorf("failed to search items: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Build result with pagination
	hasNextPage := len(items) > pageSize
	if hasNextPage {
		items = items[:pageSize]
	}

	startCursor := fmt.Sprintf("%d", offset)
	endCursor := fmt.Sprintf("%d", offset+len(items))

	total := int(totalCount)
	return &SearchResult{
		Items:           items,
		HasNextPage:     hasNextPage,
		HasPreviousPage: false,
		StartCursor:     &startCursor,
		EndCursor:       &endCursor,
		TotalCount:      &total,
	}, nil
}

// applySearchFilters parses the search query and applies filters.
func (s *SearchService) applySearchFilters(query *gorm.DB, filters []SearchFilter) *gorm.DB {
	for _, filter := range filters {
		switch filter.Type {
		case "in":
			continue

		case "is":
			switch filter.Value {
			case "read":
				query = query.Where("reading_progress_bottom_percent >= ?", 98)
			case "unread":
				query = query.Where("reading_progress_bottom_percent < ?", 2)
			case "reading":
				query = query.Where("reading_progress_bottom_percent BETWEEN ? AND ?", 2, 98)
			}

		case "label":
			query = query.Where("? = ANY(label_names)", filter.Value)

		case "type":
			switch filter.Value {
			case "pdf":
				query = query.Where("content_reader = ?", "PDF")
			case "epub":
				query = query.Where("content_reader = ?", "EPUB")
			default:
				query = query.Where("content_reader = ?", "WEB")
			}

		case "has":
			switch filter.Value {
			case "highlights":
				query = query.Where("EXISTS (SELECT 1 FROM omnivore.highlight h WHERE h.library_item_id = omnivore.library_item.id)")
			case "labels":
				query = query.Where("array_length(label_names, 1) > 0")
			}

		case "no":
			switch filter.Value {
			case "highlights":
				query = query.Where("NOT EXISTS (SELECT 1 FROM omnivore.highlight h WHERE h.library_item_id = omnivore.library_item.id)")
			case "labels":
				query = query.Where("(label_names IS NULL OR array_length(label_names, 1) = 0)")
			}

		case "fulltext":
			query = query.Where("search_tsv @@ websearch_to_tsquery('english', ?)", filter.Value)
		}
	}

	return query
}

// applySorting applies sorting based on query or defaults.
func (s *SearchService) applySorting(query *gorm.DB, searchQuery string) *gorm.DB {
	// Check for fulltext search (uses relevance ranking)
	if hasFullTextSearch(searchQuery) {
		ftQuery := extractFullTextQuery(searchQuery)
		query = query.Order(fmt.Sprintf("ts_rank_cd(search_tsv, websearch_to_tsquery('english', '%s')) DESC", ftQuery))
	}

	// Default sort: saved_at DESC NULLS LAST
	query = query.Order("saved_at DESC NULLS LAST")

	return query
}

// SearchFilter represents a parsed search filter.
type SearchFilter struct {
	Type  string
	Value string
}

// parseSearchQuery parses the search query into filters.
func parseSearchQuery(query string) []SearchFilter {
	var filters []SearchFilter

	tokens := strings.Fields(query)
	for _, token := range tokens {
		if strings.Contains(token, ":") {
			parts := strings.SplitN(token, ":", 2)
			if len(parts) == 2 {
				filterType := strings.ToLower(parts[0])
				filterValue := strings.Trim(parts[1], `"`)
				filters = append(filters, SearchFilter{
					Type:  filterType,
					Value: filterValue,
				})
			}
		} else {
			filters = append(filters, SearchFilter{
				Type:  "fulltext",
				Value: token,
			})
		}
	}

	return filters
}

func lastFilterValue(filters []SearchFilter, filterType string) string {
	value := ""
	for _, filter := range filters {
		if filter.Type == filterType {
			value = filter.Value
		}
	}
	return value
}

func hasFullTextSearch(query string) bool {
	filters := parseSearchQuery(query)
	for _, f := range filters {
		if f.Type == "fulltext" {
			return true
		}
	}
	return false
}

func extractFullTextQuery(query string) string {
	var terms []string
	filters := parseSearchQuery(query)
	for _, f := range filters {
		if f.Type == "fulltext" {
			terms = append(terms, f.Value)
		}
	}
	return strings.Join(terms, " ")
}
