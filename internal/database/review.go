package database

import (
	"database/sql"

	"nano-backend/internal/models"
)

// ========== 影视项目 (Projects) ==========

// CreateReviewProject 创建影视项目
func CreateReviewProject(project *models.ReviewProject) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec(
		"INSERT INTO review_projects (id, userId, name, coverFileId, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)",
		project.ID, project.UserID, project.Name, project.CoverFileID, project.CreatedAt, project.UpdatedAt,
	)
	return err
}

// ListReviewProjects 获取所有项目列表 (移除 userID 参数)
func ListReviewProjects() ([]models.ReviewProject, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query(
		"SELECT id, userId, name, coverFileId, createdAt, updatedAt FROM review_projects ORDER BY createdAt DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.ReviewProject
	for rows.Next() {
		var p models.ReviewProject
		var coverFileId sql.NullString
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &coverFileId, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if coverFileId.Valid {
			p.CoverFileID = coverFileId.String
		}
		// 计算集数
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM review_episodes WHERE projectId = ?",
			p.ID,
		).Scan(&p.EpisodeCount); err != nil {
			p.EpisodeCount = 0
		}
		projects = append(projects, p)
	}
	// 确保返回空切片而不是nil
	if projects == nil {
		return []models.ReviewProject{}, nil
	}
	return projects, nil
}

// GetReviewProject 获取单个项目详情 (移除 userID 参数)
func GetReviewProject(id string) (*models.ReviewProject, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var p models.ReviewProject
	var coverFileId sql.NullString
	err := db.QueryRow(
		"SELECT id, userId, name, coverFileId, createdAt, updatedAt FROM review_projects WHERE id = ?",
		id,
	).Scan(&p.ID, &p.UserID, &p.Name, &coverFileId, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if coverFileId.Valid {
		p.CoverFileID = coverFileId.String
	}
	// 计算集数
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM review_episodes WHERE projectId = ?",
		p.ID,
	).Scan(&p.EpisodeCount); err != nil {
		p.EpisodeCount = 0
	}
	return &p, nil
}

// ========== 影视单集 (Episodes) ==========

// CreateReviewEpisode 创建影视单集
func CreateReviewEpisode(episode *models.ReviewEpisode) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec(
		"INSERT INTO review_episodes (id, projectId, userId, name, coverFileId, sortOrder, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		episode.ID, episode.ProjectID, episode.UserID, episode.Name, episode.CoverFileID, episode.SortOrder, episode.CreatedAt, episode.UpdatedAt,
	)
	return err
}

// ListReviewEpisodes 获取项目的单集列表 (移除 userID 参数)
func ListReviewEpisodes(projectID string) ([]models.ReviewEpisode, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query(
		"SELECT id, projectId, userId, name, coverFileId, sortOrder, createdAt, updatedAt FROM review_episodes WHERE projectId = ? ORDER BY sortOrder ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []models.ReviewEpisode
	for rows.Next() {
		var e models.ReviewEpisode
		var coverFileId sql.NullString
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.UserID, &e.Name, &coverFileId, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if coverFileId.Valid {
			e.CoverFileID = coverFileId.String
		}
		// 计算分镜数
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM review_storyboards WHERE episodeId = ?",
			e.ID,
		).Scan(&e.StoryboardCount); err != nil {
			e.StoryboardCount = 0
		}
		episodes = append(episodes, e)
	}
	// 确保返回空切片而不是nil
	if episodes == nil {
		return []models.ReviewEpisode{}, nil
	}
	return episodes, nil
}

// GetReviewEpisode 获取单个单集详情 (移除 userID 参数)
func GetReviewEpisode(id string) (*models.ReviewEpisode, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var e models.ReviewEpisode
	var coverFileId sql.NullString
	err := db.QueryRow(
		"SELECT id, projectId, userId, name, coverFileId, sortOrder, createdAt, updatedAt FROM review_episodes WHERE id = ?",
		id,
	).Scan(&e.ID, &e.ProjectID, &e.UserID, &e.Name, &coverFileId, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if coverFileId.Valid {
		e.CoverFileID = coverFileId.String
	}

	// 计算分镜数
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM review_storyboards WHERE episodeId = ?",
		e.ID,
	).Scan(&e.StoryboardCount); err != nil {
		e.StoryboardCount = 0
	}
	return &e, nil
}

// ========== 分镜 (Storyboards) ==========

// CreateReviewStoryboard 创建分镜
func CreateReviewStoryboard(storyboard *models.ReviewStoryboard) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec(
		"INSERT INTO review_storyboards (id, episodeId, userId, imageFileId, status, feedback, sortOrder, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		storyboard.ID, storyboard.EpisodeID, storyboard.UserID, storyboard.ImageFileID, storyboard.Status, storyboard.Feedback, storyboard.SortOrder, storyboard.CreatedAt, storyboard.UpdatedAt,
	)
	return err
}

// ListReviewStoryboards 获取单集的分镜列表 (移除 userID 参数)
func ListReviewStoryboards(episodeID string) ([]models.ReviewStoryboard, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query(
		"SELECT id, episodeId, userId, imageFileId, status, feedback, sortOrder, createdAt, updatedAt FROM review_storyboards WHERE episodeId = ? ORDER BY sortOrder ASC",
		episodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var storyboards []models.ReviewStoryboard
	for rows.Next() {
		var s models.ReviewStoryboard
		var feedback sql.NullString
		if err := rows.Scan(&s.ID, &s.EpisodeID, &s.UserID, &s.ImageFileID, &s.Status, &feedback, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if feedback.Valid {
			s.Feedback = feedback.String
		}
		storyboards = append(storyboards, s)
	}
	// 确保返回空切片而不是nil
	if storyboards == nil {
		return []models.ReviewStoryboard{}, nil
	}
	return storyboards, nil
}

// GetMaxStoryboardOrder 获取当前最大排序值
func GetMaxStoryboardOrder(episodeID string) int {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var maxOrder int
	err := db.QueryRow("SELECT COALESCE(MAX(sortOrder), -1) FROM review_storyboards WHERE episodeId = ?", episodeID).Scan(&maxOrder)
	if err != nil {
		return -1
	}
	return maxOrder
}

// UpdateStoryboardStatus 更新分镜状态和反馈
func UpdateStoryboardStatus(id, status, feedback string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	_, err := db.Exec(
		"UPDATE review_storyboards SET status = ?, feedback = ?, updatedAt = ? WHERE id = ?",
		status, feedback, now, id,
	)
	return err
}

// UpdateStoryboardOrder 批量更新排序
func UpdateStoryboardOrder(storyboardIDs []string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	for i, id := range storyboardIDs {
		_, err := db.Exec(
			"UPDATE review_storyboards SET sortOrder = ?, updatedAt = ? WHERE id = ?",
			i, now, id,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetMaxEpisodeOrder 获取当前项目下单集的最大排序值
func GetMaxEpisodeOrder(projectID string) int {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var maxOrder int
	// 使用 COALESCE 处理没有记录的情况，默认返回 -1
	err := db.QueryRow("SELECT COALESCE(MAX(sortOrder), -1) FROM review_episodes WHERE projectId = ?", projectID).Scan(&maxOrder)
	if err != nil {
		return -1
	}
	return maxOrder
}

// UpdateEpisodeOrder 批量更新单集排序
func UpdateEpisodeOrder(episodeIDs []string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	for i, id := range episodeIDs {
		_, err := db.Exec(
			"UPDATE review_episodes SET sortOrder = ?, updatedAt = ? WHERE id = ?",
			i, now, id,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateReviewProject 更新影视项目
func UpdateReviewProject(projectID, name, coverFileID string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	if coverFileID != "" {
		_, err := db.Exec(
			"UPDATE review_projects SET name = ?, coverFileId = ?, updatedAt = ? WHERE id = ?",
			name, coverFileID, now, projectID,
		)
		return err
	}
	_, err := db.Exec(
		"UPDATE review_projects SET name = ?, updatedAt = ? WHERE id = ?",
		name, now, projectID,
	)
	return err
}

// UpdateReviewEpisode 更新影视单集
func UpdateReviewEpisode(episodeID, name, coverFileID string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	if coverFileID != "" {
		_, err := db.Exec(
			"UPDATE review_episodes SET name = ?, coverFileId = ?, updatedAt = ? WHERE id = ?",
			name, coverFileID, now, episodeID,
		)
		return err
	}
	_, err := db.Exec(
		"UPDATE review_episodes SET name = ?, updatedAt = ? WHERE id = ?",
		name, now, episodeID,
	)
	return err
}

// UpdateReviewStoryboard 更新分镜
func UpdateReviewStoryboard(storyboardID, name, imageFileID string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	if imageFileID != "" {
		_, err := db.Exec(
			"UPDATE review_storyboards SET name = ?, imageFileId = ?, status = 'pending', feedback = '', updatedAt = ? WHERE id = ?",
			name, imageFileID, now, storyboardID,
		)
		return err
	}
	_, err := db.Exec(
		"UPDATE review_storyboards SET name = ?, status = 'pending', feedback = '', updatedAt = ? WHERE id = ?",
		name, now, storyboardID,
	)
	return err
}

// GetReviewStoryboard 获取单个分镜详情 (移除 userID 参数)
func GetReviewStoryboard(id string) (*models.ReviewStoryboard, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var s models.ReviewStoryboard
	var feedback sql.NullString
	err := db.QueryRow(
		"SELECT id, episodeId, userId, imageFileId, status, feedback, sortOrder, createdAt, updatedAt FROM review_storyboards WHERE id = ?",
		id,
	).Scan(&s.ID, &s.EpisodeID, &s.UserID, &s.ImageFileID, &s.Status, &feedback, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if feedback.Valid {
		s.Feedback = feedback.String
	}

	return &s, nil
}

// ========== 删除操作 ==========

// DeleteReviewStoryboard 删除分镜
func DeleteReviewStoryboard(id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM review_storyboards WHERE id = ?", id)
	return err
}

// DeleteReviewEpisode 删除单集 (包含其下的所有分镜)
func DeleteReviewEpisode(id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 删除该单集下的所有分镜
	if _, err := tx.Exec("DELETE FROM review_storyboards WHERE episodeId = ?", id); err != nil {
		return err
	}

	// 2. 删除单集本身
	if _, err := tx.Exec("DELETE FROM review_episodes WHERE id = ?", id); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteReviewProject 删除项目 (包含其下的所有单集和分镜)
func DeleteReviewProject(id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 删除该项目下所有单集的分镜
	queryDeleteStoryboards := `
		DELETE FROM review_storyboards
		WHERE episodeId IN (SELECT id FROM review_episodes WHERE projectId = ?)
	`
	if _, err := tx.Exec(queryDeleteStoryboards, id); err != nil {
		return err
	}

	// 2. 删除该项目下的所有单集
	if _, err := tx.Exec("DELETE FROM review_episodes WHERE projectId = ?", id); err != nil {
		return err
	}

	// 3. 删除项目本身
	if _, err := tx.Exec("DELETE FROM review_projects WHERE id = ?", id); err != nil {
		return err
	}

	return tx.Commit()
}
