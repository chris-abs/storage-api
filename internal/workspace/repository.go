package workspace

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/chrisabs/storage/internal/models"
)

type Repository struct {
    db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
    return &Repository{db: db}
}

func (r *Repository) Create(workspace *models.Workspace) error {
    query := `
        INSERT INTO workspace (id, name, description, user_id, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id`

    err := r.db.QueryRow(
        query,
        workspace.ID,
        workspace.Name,
        workspace.Description,
        workspace.UserID,
        workspace.CreatedAt,
        workspace.UpdatedAt,
    ).Scan(&workspace.ID)

    if err != nil {
        return fmt.Errorf("error creating workspace: %v", err)
    }

    return nil
}

func (r *Repository) GetByID(id int) (*models.Workspace, error) {
    workspaceQuery := `
        SELECT w.id, w.name, w.description, w.user_id, w.created_at, w.updated_at
        FROM workspace w
        WHERE w.id = $1`

    workspace := new(models.Workspace)
    err := r.db.QueryRow(workspaceQuery, id).Scan(
        &workspace.ID,
        &workspace.Name,
        &workspace.Description,
        &workspace.UserID,
        &workspace.CreatedAt,
        &workspace.UpdatedAt,
    )

    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("workspace not found")
    }
    if err != nil {
        return nil, err
    }

    containersQuery := `
        SELECT id, name, qr_code, qr_code_image, number, location, 
               user_id, created_at, updated_at
        FROM container
        WHERE workspace_id = $1
        ORDER BY created_at DESC`

    rows, err := r.db.Query(containersQuery, id)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    workspace.Containers = make([]models.Container, 0)
    for rows.Next() {
        var container models.Container
        err := rows.Scan(
            &container.ID,
            &container.Name,
            &container.QRCode,
            &container.QRCodeImage,
            &container.Number,
            &container.Location,
            &container.UserID,
            &container.CreatedAt,
            &container.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        workspace.Containers = append(workspace.Containers, container)
    }

    return workspace, nil
}

func (r *Repository) GetByUserID(userID int) ([]*models.Workspace, error) {
    query := `
        SELECT id, name, description, user_id, created_at, updated_at 
        FROM workspace
        WHERE user_id = $1
        ORDER BY created_at DESC`

    rows, err := r.db.Query(query, userID)
    if err != nil {
        return nil, fmt.Errorf("error querying workspaces: %v", err)
    }
    defer rows.Close()

    var workspaces []*models.Workspace
    for rows.Next() {
        workspace := new(models.Workspace)
        err := rows.Scan(
            &workspace.ID,
            &workspace.Name,
            &workspace.Description,
            &workspace.UserID,
            &workspace.CreatedAt,
            &workspace.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("error scanning workspace: %v", err)
        }

        containersQuery := `
            SELECT id, name, qr_code, qr_code_image, number, location, 
                   user_id, created_at, updated_at
            FROM container
            WHERE workspace_id = $1
            ORDER BY created_at DESC`

        containerRows, err := r.db.Query(containersQuery, workspace.ID)
        if err != nil {
            return nil, fmt.Errorf("error querying containers: %v", err)
        }

        workspace.Containers = make([]models.Container, 0)
        func() {
            defer containerRows.Close()
            for containerRows.Next() {
                var container models.Container
                err := containerRows.Scan(
                    &container.ID,
                    &container.Name,
                    &container.QRCode,
                    &container.QRCodeImage,
                    &container.Number,
                    &container.Location,
                    &container.UserID,
                    &container.CreatedAt,
                    &container.UpdatedAt,
                )
                if err != nil {
                    return
                }
                workspace.Containers = append(workspace.Containers, container)
            }
        }()

        workspaces = append(workspaces, workspace)
    }

    return workspaces, nil
}

func (r *Repository) Update(workspace *models.Workspace) error {
    query := `
        UPDATE workspace
        SET name = $2, description = $3, updated_at = $4
        WHERE id = $1`

    result, err := r.db.Exec(
        query,
        workspace.ID,
        workspace.Name,
        workspace.Description,
        time.Now().UTC(),
    )
    if err != nil {
        return fmt.Errorf("error updating workspace: %v", err)
    }

    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("error checking update result: %v", err)
    }

    if rowsAffected == 0 {
        return fmt.Errorf("workspace not found")
    }

    return nil
}

func (r *Repository) Delete(id int) error {
    query := `DELETE FROM workspace WHERE id = $1`
    result, err := r.db.Exec(query, id)
    if err != nil {
        return fmt.Errorf("error deleting workspace: %v", err)
    }

    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("error checking delete result: %v", err)
    }

    if rowsAffected == 0 {
        return fmt.Errorf("workspace not found")
    }

    return nil
}