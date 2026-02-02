package handlers

import (
	"io"
	"log"

	"nano-backend/internal/database"
	"nano-backend/internal/middleware"
	"nano-backend/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ========== 影视项目 (Projects) ==========

// CreateReviewProject 创建影视项目
func CreateReviewProject(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	name := c.FormValue("name")

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "项目名称不能为空"})
	}

	// 处理封面上传 (非必要)
	var coverFileID string
	fileHeader, err := c.FormFile("cover")
	if err == nil {
		// 复用现有的文件保存逻辑
		file, _ := fileHeader.Open()
		buf, _ := io.ReadAll(file)
		savedFile, err := SaveBufferToFile(user.ID, "project-cover", fileHeader.Header.Get("Content-Type"), fileHeader.Filename, buf, true)
		if err == nil {
			coverFileID = savedFile.ID
		}
	}

	now := models.Now()
	project := &models.ReviewProject{
		ID:          uuid.New().String(),
		UserID:      user.ID,
		Name:        name,
		CoverFileID: coverFileID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := database.CreateReviewProject(project); err != nil {
		log.Printf("[review] Error creating project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "创建失败"})
	}

	return c.JSON(project)
}

// ListReviewProjects 获取项目列表
func ListReviewProjects(c *fiber.Ctx) error {
	projects, err := database.ListReviewProjects()
	if err != nil {
		log.Printf("[review] Error listing projects: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(projects)
}

// GetReviewProject 获取项目详情
func GetReviewProject(c *fiber.Ctx) error {
	id := c.Params("id")

	project, err := database.GetReviewProject(id)
	if err != nil {
		log.Printf("[review] Error getting project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if project == nil {
		return c.Status(404).JSON(fiber.Map{"error": "项目不存在"})
	}

	return c.JSON(project)
}

// ========== 影视单集 (Episodes) ==========

// CreateReviewEpisode 创建单集
func CreateReviewEpisode(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	projectID := c.Params("projectId")
	name := c.FormValue("name")

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "单集名称不能为空"})
	}

	// 验证项目存在
	project, err := database.GetReviewProject(projectID)
	if err != nil {
		log.Printf("[review] Error getting project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if project == nil {
		return c.Status(404).JSON(fiber.Map{"error": "项目不存在"})
	}

	// 处理封面上传 (非必要)
	var coverFileID string
	fileHeader, err := c.FormFile("cover")
	if err == nil {
		file, _ := fileHeader.Open()
		buf, _ := io.ReadAll(file)
		savedFile, err := SaveBufferToFile(user.ID, "episode-cover", fileHeader.Header.Get("Content-Type"), fileHeader.Filename, buf, true)
		if err == nil {
			coverFileID = savedFile.ID
		}
	}

	// 获取当前最大排序值
	maxOrder := database.GetMaxEpisodeOrder(projectID)

	now := models.Now()
	episode := &models.ReviewEpisode{
		ID:          uuid.New().String(),
		ProjectID:   projectID,
		UserID:      user.ID,
		Name:        name,
		CoverFileID: coverFileID,
		SortOrder:   maxOrder + 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := database.CreateReviewEpisode(episode); err != nil {
		log.Printf("[review] Error creating episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "创建失败"})
	}

	return c.JSON(episode)
}

// ListReviewEpisodes 获取单集列表
func ListReviewEpisodes(c *fiber.Ctx) error {
	projectID := c.Params("projectId")

	episodes, err := database.ListReviewEpisodes(projectID)
	if err != nil {
		log.Printf("[review] Error listing episodes: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(episodes)
}

// GetReviewEpisode 获取单集详情
func GetReviewEpisode(c *fiber.Ctx) error {
	id := c.Params("id")

	episode, err := database.GetReviewEpisode(id)
	if err != nil {
		log.Printf("[review] Error getting episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if episode == nil {
		return c.Status(404).JSON(fiber.Map{"error": "单集不存在"})
	}

	return c.JSON(episode)
}

// ========== 分镜 (Storyboards) ==========

// CreateReviewStoryboard 创建分镜
func CreateReviewStoryboard(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	episodeID := c.Params("episodeId")
	name := c.FormValue("name")

	// 验证单集存在
	episode, err := database.GetReviewEpisode(episodeID)
	if err != nil {
		log.Printf("[review] Error getting episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if episode == nil {
		return c.Status(404).JSON(fiber.Map{"error": "单集不存在"})
	}

	// 处理分镜图片 (必要)
	fileHeader, err := c.FormFile("image")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "必须上传分镜图片"})
	}

	// 读取并保存图片
	file, _ := fileHeader.Open()
	buf, _ := io.ReadAll(file)
	savedFile, err := SaveBufferToFile(user.ID, "storyboard-image", fileHeader.Header.Get("Content-Type"), fileHeader.Filename, buf, true)
	if err != nil {
		log.Printf("[review] Error saving storyboard image: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "图片保存失败"})
	}

	// 获取当前最大排序值以便追加到末尾
	maxOrder := database.GetMaxStoryboardOrder(episodeID)

	now := models.Now()
	storyboard := &models.ReviewStoryboard{
		ID:          uuid.New().String(),
		EpisodeID:   episodeID,
		UserID:      user.ID,
		Name:        name,
		ImageFileID: savedFile.ID,
		Status:      "pending", // 默认为灰色/未审阅
		SortOrder:   maxOrder + 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := database.CreateReviewStoryboard(storyboard); err != nil {
		log.Printf("[review] Error creating storyboard: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "创建失败"})
	}

	return c.JSON(storyboard)
}

// ListReviewStoryboards 获取分镜列表
func ListReviewStoryboards(c *fiber.Ctx) error {
	episodeID := c.Params("episodeId")
	token := middleware.GetToken(c)

	storyboards, err := database.ListReviewStoryboards(episodeID)
	if err != nil {
		log.Printf("[review] Error listing storyboards: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	// 构建响应，包含图片URL
	responses := make([]models.ReviewStoryboardResponse, len(storyboards))
	for i, sb := range storyboards {
		responses[i] = models.ReviewStoryboardResponse{
			ReviewStoryboard: sb,
		}
		// 获取图片URL
		if sb.ImageFileID != "" {
			if file, err := database.GetFileByID(sb.ImageFileID); err == nil && file != nil {
				responses[i].ImageURL = buildClientFileURL(file.ID, token, false)
			}
		}
	}

	return c.JSON(responses)
}

// ReviewStoryboard 审阅/修改分镜状态
func ReviewStoryboard(c *fiber.Ctx) error {
	storyboardID := c.Params("id")

	var body struct {
		Status   string `json:"status"`   // "approved" 或 "rejected"
		Feedback string `json:"feedback"` // 当 rejected 时必填
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "格式错误"})
	}

	if body.Status == "rejected" && body.Feedback == "" {
		return c.Status(400).JSON(fiber.Map{"error": "未通过时必须填写修改建议"})
	}

	// 验证权限
	storyboard, err := database.GetReviewStoryboard(storyboardID)
	if err != nil {
		log.Printf("[review] Error getting storyboard: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if storyboard == nil {
		return c.Status(404).JSON(fiber.Map{"error": "分镜不存在"})
	}

	if err := database.UpdateStoryboardStatus(storyboardID, body.Status, body.Feedback); err != nil {
		log.Printf("[review] Error updating storyboard status: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "更新失败"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ReorderStoryboards 分镜排序 (拖拽后调用)
func ReorderStoryboards(c *fiber.Ctx) error {

	// 接收一个有序的ID列表
	var body struct {
		StoryboardIDs []string `json:"storyboardIds"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "格式错误"})
	}

	if len(body.StoryboardIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "分镜ID列表不能为空"})
	}

	// 验证权限
	for _, id := range body.StoryboardIDs {
		storyboard, err := database.GetReviewStoryboard(id)
		if err != nil || storyboard == nil {
			return c.Status(404).JSON(fiber.Map{"error": "分镜不存在或无权限访问"})
		}
	}

	// 批量更新排序
	if err := database.UpdateStoryboardOrder(body.StoryboardIDs); err != nil {
		log.Printf("[review] Error updating storyboard order: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "排序更新失败"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ReorderEpisodes 单集排序
func ReorderEpisodes(c *fiber.Ctx) error {
	var body struct {
		EpisodeIDs []string `json:"episodeIds"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "格式错误"})
	}

	if len(body.EpisodeIDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ID列表不能为空"})
	}

	// 简单的权限验证：检查这些单集是否存在
	for _, id := range body.EpisodeIDs {
		ep, err := database.GetReviewEpisode(id)
		if err != nil || ep == nil {
			return c.Status(404).JSON(fiber.Map{"error": "单集不存在或无权限访问"})
		}
	}

	// 批量更新排序
	if err := database.UpdateEpisodeOrder(body.EpisodeIDs); err != nil {
		log.Printf("[review] Error updating episode order: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "排序更新失败"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// UpdateReviewProject 更新影视项目
func UpdateReviewProject(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	projectID := c.Params("id")
	name := c.FormValue("name")

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "项目名称不能为空"})
	}

	// 1. 获取原数据
	existing, err := database.GetReviewProject(projectID)
	if err != nil {
		log.Printf("[review] Error getting project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if existing == nil {
		return c.Status(404).JSON(fiber.Map{"error": "项目不存在"})
	}

	// 2. 权限校验：非创建者且非管理则报错
	if existing.UserID != user.ID && user.Role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "无权修改他人的项目"})
	}

	// 3. 处理封面上传 (可选)
	var coverFileID string
	fileHeader, err := c.FormFile("cover")
	if err == nil {
		file, _ := fileHeader.Open()
		buf, _ := io.ReadAll(file)
		savedFile, err := SaveBufferToFile(user.ID, "project-cover", fileHeader.Header.Get("Content-Type"), fileHeader.Filename, buf, true)
		if err == nil {
			coverFileID = savedFile.ID
		}
	}

	// 4. 更新数据
	if err := database.UpdateReviewProject(projectID, name, coverFileID); err != nil {
		log.Printf("[review] Error updating project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "更新失败"})
	}

	// 5. 返回更新后的项目
	updatedProject, err := database.GetReviewProject(projectID)
	if err != nil {
		log.Printf("[review] Error getting updated project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(updatedProject)
}

// UpdateReviewEpisode 更新影视单集
func UpdateReviewEpisode(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	episodeID := c.Params("id")
	name := c.FormValue("name")

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "单集名称不能为空"})
	}

	// 1. 获取原数据
	existing, err := database.GetReviewEpisode(episodeID)
	if err != nil {
		log.Printf("[review] Error getting episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if existing == nil {
		return c.Status(404).JSON(fiber.Map{"error": "单集不存在"})
	}

	// 2. 权限校验：非创建者且非管理则报错
	if existing.UserID != user.ID && user.Role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "无权修改他人的单集"})
	}

	// 3. 处理封面上传 (可选)
	var coverFileID string
	fileHeader, err := c.FormFile("cover")
	if err == nil {
		file, _ := fileHeader.Open()
		buf, _ := io.ReadAll(file)
		savedFile, err := SaveBufferToFile(user.ID, "episode-cover", fileHeader.Header.Get("Content-Type"), fileHeader.Filename, buf, true)
		if err == nil {
			coverFileID = savedFile.ID
		}
	}

	// 4. 更新数据
	if err := database.UpdateReviewEpisode(episodeID, name, coverFileID); err != nil {
		log.Printf("[review] Error updating episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "更新失败"})
	}

	// 5. 返回更新后的单集
	updatedEpisode, err := database.GetReviewEpisode(episodeID)
	if err != nil {
		log.Printf("[review] Error getting updated episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(updatedEpisode)
}

// UpdateReviewStoryboard 更新分镜
func UpdateReviewStoryboard(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	storyboardID := c.Params("id")
	name := c.FormValue("name")

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "分镜名称不能为空"})
	}

	// 1. 获取原数据
	existing, err := database.GetReviewStoryboard(storyboardID)
	if err != nil {
		log.Printf("[review] Error getting storyboard: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if existing == nil {
		return c.Status(404).JSON(fiber.Map{"error": "分镜不存在"})
	}

	// 2. 权限校验：非创建者且非管理则报错
	if existing.UserID != user.ID && user.Role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "无权修改他人的分镜"})
	}

	// 3. 处理分镜图片 (可选)
	var imageFileID string
	fileHeader, err := c.FormFile("image")
	if err == nil {
		file, _ := fileHeader.Open()
		buf, _ := io.ReadAll(file)
		savedFile, err := SaveBufferToFile(user.ID, "storyboard-image", fileHeader.Header.Get("Content-Type"), fileHeader.Filename, buf, true)
		if err != nil {
			log.Printf("[review] Error saving storyboard image: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "图片保存失败"})
		}
		imageFileID = savedFile.ID
	}

	// 4. 更新数据并强制重置状态
	if err := database.UpdateReviewStoryboard(storyboardID, name, imageFileID); err != nil {
		log.Printf("[review] Error updating storyboard: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "更新失败"})
	}

	// 5. 返回更新后的分镜
	updatedStoryboard, err := database.GetReviewStoryboard(storyboardID)
	if err != nil {
		log.Printf("[review] Error getting updated storyboard: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(updatedStoryboard)
}

// ========== 删除 Handlers ==========

// DeleteReviewProject 删除项目
func DeleteReviewProject(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	projectID := c.Params("id")

	// 1. 获取项目信息进行权限验证
	project, err := database.GetReviewProject(projectID)
	if err != nil {
		log.Printf("[review] Error getting project for delete: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if project == nil {
		return c.Status(404).JSON(fiber.Map{"error": "项目不存在"})
	}

	// 2. 权限校验
	if project.UserID != user.ID && user.Role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "无权删除他人的项目"})
	}

	// 3. 执行删除
	if err := database.DeleteReviewProject(projectID); err != nil {
		log.Printf("[review] Error deleting project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "删除失败"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// DeleteReviewEpisode 删除单集
func DeleteReviewEpisode(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	episodeID := c.Params("id")

	// 1. 获取单集信息进行权限验证
	episode, err := database.GetReviewEpisode(episodeID)
	if err != nil {
		log.Printf("[review] Error getting episode for delete: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if episode == nil {
		return c.Status(404).JSON(fiber.Map{"error": "单集不存在"})
	}

	// 2. 权限校验
	if episode.UserID != user.ID && user.Role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "无权删除他人的单集"})
	}

	// 3. 执行删除
	if err := database.DeleteReviewEpisode(episodeID); err != nil {
		log.Printf("[review] Error deleting episode: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "删除失败"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// DeleteReviewStoryboard 删除分镜
func DeleteReviewStoryboard(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	storyboardID := c.Params("id")

	// 1. 获取分镜信息进行权限验证
	storyboard, err := database.GetReviewStoryboard(storyboardID)
	if err != nil {
		log.Printf("[review] Error getting storyboard for delete: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if storyboard == nil {
		return c.Status(404).JSON(fiber.Map{"error": "分镜不存在"})
	}

	// 2. 权限校验
	if storyboard.UserID != user.ID && user.Role != "admin" {
		return c.Status(403).JSON(fiber.Map{"error": "无权删除他人的分镜"})
	}

	// 3. 执行删除
	if err := database.DeleteReviewStoryboard(storyboardID); err != nil {
		log.Printf("[review] Error deleting storyboard: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "删除失败"})
	}

	return c.JSON(fiber.Map{"ok": true})
}
