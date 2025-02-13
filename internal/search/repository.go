package search

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/chrisabs/storage/internal/models"
)

type Repository struct {
    db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
    return &Repository{db: db}
}

func (r *Repository) Search(query string, userID int) (*SearchResponse, error) {
    sqlQuery := `
    WITH workspace_matches AS (
        SELECT 
            'workspace' as type,
            id,
            name,
            description,
            CASE
                WHEN name ILIKE $1 THEN 1.0
                WHEN name ILIKE $1 || '%' THEN 0.8
                WHEN name ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', name || ' ' || COALESCE(description, '')), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank,
            NULL as container_name,
            name as workspace_name,
            NULL as colour
        FROM workspace 
        WHERE 
            user_id = $2 AND
            (
                name ILIKE $1 OR
                to_tsvector('english', name || ' ' || COALESCE(description, '')) @@ 
                websearch_to_tsquery('english', $1)
            )
    ),
    container_matches AS (
        SELECT 
            'container' as type,
            c.id,
            c.name,
            COALESCE(c.location, '') as description,
            CASE
                WHEN c.name ILIKE $1 THEN 1.0
                WHEN c.name ILIKE $1 || '%' THEN 0.8
                WHEN c.name ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', c.name), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank,
            NULL as container_name,
            w.name as workspace_name,
            NULL as colour
        FROM container c
        LEFT JOIN workspace w ON c.workspace_id = w.id
        WHERE 
            c.user_id = $2 AND
            (
                c.name ILIKE $1 OR
                to_tsvector('english', c.name) @@ websearch_to_tsquery('english', $1)
            )
    ),
    item_matches AS (
        SELECT 
            'item' as type,
            i.id,
            i.name,
            i.description,
            CASE
                WHEN i.name ILIKE $1 THEN 1.0
                WHEN i.name ILIKE $1 || '%' THEN 0.8
                WHEN i.name ILIKE '%' || $1 || '%' OR i.description ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', i.name || ' ' || COALESCE(i.description, '')), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank,
            c.name as container_name,
            NULL as workspace_name,
            NULL as colour
        FROM item i
        LEFT JOIN container c ON i.container_id = c.id
        WHERE 
            c.user_id = $2 AND
            (
                i.name ILIKE $1 OR
                i.description ILIKE '%' || $1 || '%' OR
                to_tsvector('english', i.name || ' ' || COALESCE(i.description, '')) @@ 
                websearch_to_tsquery('english', $1)
            )
    ),
    tag_matches AS (
        SELECT DISTINCT
            'tag' as type,
            t.id,
            t.name,
            '' as description,
            CASE
                WHEN t.name ILIKE $1 THEN 1.0
                WHEN t.name ILIKE $1 || '%' THEN 0.8
                WHEN t.name ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', t.name), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank,
            NULL as container_name,
            NULL as workspace_name,
            t.colour
        FROM tag t
        WHERE t.name ILIKE $1 OR t.name ILIKE $1 || '%' OR t.name ILIKE '%' || $1 || '%'
    ),
    tagged_items AS (
        SELECT DISTINCT
            'tagged_item' as type,
            i.id,
            i.name,
            i.description,
            CASE
                WHEN t.name ILIKE $1 THEN 0.9
                WHEN t.name ILIKE $1 || '%' THEN 0.7
                WHEN t.name ILIKE '%' || $1 || '%' THEN 0.5
                ELSE COALESCE(ts_rank(to_tsvector('english', t.name), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank,
            c.name as container_name,
            w.name as workspace_name,
            NULL as colour
        FROM item i
        INNER JOIN item_tag it ON i.id = it.item_id
        INNER JOIN tag t ON it.tag_id = t.id
        LEFT JOIN container c ON i.container_id = c.id
        LEFT JOIN workspace w ON c.workspace_id = w.id
        WHERE 
            (c.user_id = $2 OR i.container_id IS NULL) AND
            (
                t.name ILIKE $1 OR
                t.name ILIKE $1 || '%' OR
                t.name ILIKE '%' || $1 || '%' OR
                to_tsvector('english', t.name) @@ websearch_to_tsquery('english', $1)
            ) AND
            i.id NOT IN (SELECT id FROM item_matches)
    )
    SELECT type, id, name, description, rank, container_name, workspace_name, colour 
    FROM (
        SELECT * FROM workspace_matches
        UNION ALL
        SELECT * FROM container_matches
        UNION ALL
        SELECT * FROM item_matches
        UNION ALL
        SELECT * FROM tag_matches
        UNION ALL
        SELECT * FROM tagged_items
    ) combined_results
    ORDER BY rank DESC;`

    rows, err := r.db.Query(sqlQuery, query, userID)
    if err != nil {
        return nil, fmt.Errorf("error executing search: %v", err)
    }
    defer rows.Close()

    response := &SearchResponse{
        Workspaces:  make([]SearchResult, 0),
        Containers:  make([]SearchResult, 0),
        Items:       make([]SearchResult, 0),
        Tags:        make([]SearchResult, 0),
        TaggedItems: make([]SearchResult, 0),
    }

    for rows.Next() {
        var result SearchResult
        var containerName, workspaceName, colour sql.NullString
        err := rows.Scan(
            &result.Type,
            &result.ID,
            &result.Name,
            &result.Description,
            &result.Rank,
            &containerName,
            &workspaceName,
            &colour,
        )
        if err != nil {
            return nil, fmt.Errorf("error scanning search result: %v", err)
        }
    
        if containerName.Valid {
            result.ContainerName = &containerName.String
        }
        if workspaceName.Valid {
            result.WorkspaceName = &workspaceName.String
        }
        if colour.Valid {
            result.Colour = &colour.String
        }

        switch result.Type {
        case "workspace":
            response.Workspaces = append(response.Workspaces, result)
        case "container":
            response.Containers = append(response.Containers, result)
        case "item":
            response.Items = append(response.Items, result)
        case "tag":
            response.Tags = append(response.Tags, result)
        case "tagged_item":
            response.TaggedItems = append(response.TaggedItems, result)
        }
    }

    return response, nil
}

func (r *Repository) SearchWorkspaces(query string, userID int) (WorkspaceSearchResults, error) {
    sqlQuery := `
        SELECT 
            w.id, w.name, w.description, w.user_id, w.created_at, w.updated_at,
            CASE
                WHEN w.name ILIKE $1 THEN 1.0
                WHEN w.name ILIKE $1 || '%' THEN 0.8
                WHEN w.name ILIKE '%' || $1 || '%' OR w.description ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', w.name || ' ' || COALESCE(w.description, '')), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank
        FROM workspace w
        WHERE 
            w.user_id = $2 AND
            (
                w.name ILIKE $1 OR
                w.description ILIKE '%' || $1 || '%' OR
                to_tsvector('english', w.name || ' ' || COALESCE(w.description, '')) @@ 
                websearch_to_tsquery('english', $1)
            )
        ORDER BY rank DESC;`

    rows, err := r.db.Query(sqlQuery, query, userID)
    if err != nil {
        return nil, fmt.Errorf("error executing workspace search: %v", err)
    }
    defer rows.Close()

    var results WorkspaceSearchResults
    for rows.Next() {
        var result WorkspaceSearchResult
        err := rows.Scan(
            &result.ID,
            &result.Name,
            &result.Description,
            &result.UserID,
            &result.CreatedAt,
            &result.UpdatedAt,
            &result.Rank,
        )
        if err != nil {
            return nil, fmt.Errorf("error scanning workspace search result: %v", err)
        }

        results = append(results, result)
    }

    return results, nil
}

func (r *Repository) SearchContainers(query string, userID int) (ContainerSearchResults, error) {
    sqlQuery := `
        SELECT 
            c.id, c.name, c.qr_code, c.qr_code_image, c.number, c.location,
            c.user_id, c.workspace_id, c.created_at, c.updated_at,
            CASE
                WHEN c.name ILIKE $1 THEN 1.0
                WHEN c.name ILIKE $1 || '%' THEN 0.8
                WHEN c.name ILIKE '%' || $1 || '%' OR c.location ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', c.name || ' ' || COALESCE(c.location, '')), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank
        FROM container c
        WHERE 
            c.user_id = $2 AND
            (
                c.name ILIKE $1 OR
                c.location ILIKE '%' || $1 || '%' OR
                to_tsvector('english', c.name || ' ' || COALESCE(c.location, '')) @@ 
                websearch_to_tsquery('english', $1)
            )
        ORDER BY rank DESC;`

    rows, err := r.db.Query(sqlQuery, query, userID)
    if err != nil {
        return nil, fmt.Errorf("error executing container search: %v", err)
    }
    defer rows.Close()

    var results ContainerSearchResults
    for rows.Next() {
        var result ContainerSearchResult
        err := rows.Scan(
            &result.ID,
            &result.Name,
            &result.QRCode,
            &result.QRCodeImage,
            &result.Number,
            &result.Location,
            &result.UserID,
            &result.WorkspaceID,
            &result.CreatedAt,
            &result.UpdatedAt,
            &result.Rank,
        )
        if err != nil {
            return nil, fmt.Errorf("error scanning container search result: %v", err)
        }

        results = append(results, result)
    }

    return results, nil
}

func (r *Repository) SearchItems(query string, userID int) (ItemSearchResults, error) {
    sqlQuery := `
        SELECT 
            i.id, i.name, i.description, i.quantity, i.container_id,
            i.created_at, i.updated_at,
            CASE
                WHEN i.name ILIKE $1 THEN 1.0
                WHEN i.name ILIKE $1 || '%' THEN 0.8
                WHEN i.name ILIKE '%' || $1 || '%' OR i.description ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', i.name || ' ' || COALESCE(i.description, '')), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank
        FROM item i
        JOIN container c ON i.container_id = c.id
        WHERE 
            c.user_id = $2 AND
            (
                i.name ILIKE $1 OR
                i.description ILIKE '%' || $1 || '%' OR
                to_tsvector('english', i.name || ' ' || COALESCE(i.description, '')) @@ 
                websearch_to_tsquery('english', $1)
            )
        ORDER BY rank DESC;`

    rows, err := r.db.Query(sqlQuery, query, userID)
    if err != nil {
        return nil, fmt.Errorf("error executing item search: %v", err)
    }
    defer rows.Close()

    var results ItemSearchResults
    for rows.Next() {
        var result ItemSearchResult
        err := rows.Scan(
            &result.ID,
            &result.Name,
            &result.Description,
            &result.Quantity,
            &result.ContainerID,
            &result.CreatedAt,
            &result.UpdatedAt,
            &result.Rank,
        )
        if err != nil {
            return nil, fmt.Errorf("error scanning item search result: %v", err)
        }

        results = append(results, result)
    }

    return results, nil
}

func (r *Repository) SearchTags(query string, userID int) (TagSearchResults, error) {
    sqlQuery := `
        SELECT DISTINCT
            t.id, t.name, t.colour, t.created_at, t.updated_at,
            CASE
                WHEN t.name ILIKE $1 THEN 1.0
                WHEN t.name ILIKE $1 || '%' THEN 0.8
                WHEN t.name ILIKE '%' || $1 || '%' THEN 0.6
                ELSE COALESCE(ts_rank(to_tsvector('english', t.name), 
                    websearch_to_tsquery('english', $1)), 0.0)
            END as rank
        FROM tag t
        JOIN item_tag it ON t.id = it.tag_id
        JOIN item i ON it.item_id = i.id
        JOIN container c ON i.container_id = c.id
        WHERE 
            c.user_id = $2 AND
            (
                t.name ILIKE $1 OR
                to_tsvector('english', t.name) @@ websearch_to_tsquery('english', $1)
            )
        ORDER BY rank DESC;`

    rows, err := r.db.Query(sqlQuery, query, userID)
    if err != nil {
        return nil, fmt.Errorf("error executing tag search: %v", err)
    }
    defer rows.Close()

    var results TagSearchResults
    for rows.Next() {
        var result TagSearchResult
        err := rows.Scan(
            &result.ID,
            &result.Name,
            &result.Colour,
            &result.CreatedAt,
            &result.UpdatedAt,
            &result.Rank,
        )
        if err != nil {
            return nil, fmt.Errorf("error scanning tag search result: %v", err)
        }

        results = append(results, result)
    }

    return results, nil
}

func (r *Repository) FindContainerByQR(qrCode string, userID int) (*models.Container, error) {
   query := `
       SELECT 
           c.*,
           jsonb_build_object(
               'id', w.id,
               'name', w.name,
               'description', w.description,
               'userId', w.user_id,
               'createdAt', w.created_at,
               'updatedAt', w.updated_at
           ) as workspace
       FROM container c
       LEFT JOIN workspace w ON c.workspace_id = w.id
       WHERE c.qr_code = $1 AND c.user_id = $2
       LIMIT 1`

   container := new(models.Container)
   var workspaceJSON []byte
   
   err := r.db.QueryRow(query, qrCode, userID).Scan(
       &container.ID,
       &container.Name,
       &container.QRCode,
       &container.QRCodeImage,
       &container.Number,
       &container.Location,
       &container.UserID,
       &container.WorkspaceID,
       &container.CreatedAt,
       &container.UpdatedAt,
       &workspaceJSON,
   )

   if err == sql.ErrNoRows {
       return nil, fmt.Errorf("container not found")
   }
   if err != nil {
       return nil, fmt.Errorf("error finding container: %v", err)
   }

   if len(workspaceJSON) > 0 {
       if err := json.Unmarshal(workspaceJSON, &container.Workspace); err != nil {
           return nil, fmt.Errorf("error unmarshaling workspace: %v", err)
       }
   }

   return container, nil
}
       