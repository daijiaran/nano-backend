# Nano Banana Pro Studio - 项目架构分析文档

## 项目概述

本项目是一个基于 Go 后端和 React 前端的 AI 图像/视频生成平台，支持用户登录、图片生成、视频生成、素材库管理等功能。

### 技术栈

**后端：**
- Go 1.x
- Fiber Web 框架
- SQLite 数据库
- GORM ORM

**前端：**
- React 19
- TypeScript
- Vite
- Tailwind CSS 4

---

## 一、后端 API 列表

### 1. 健康检查

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/health` | 否 | 健康检查接口 |

---

### 2. 认证相关 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/auth/login` | 否 | 用户登录 |
| POST | `/api/auth/logout` | 是 | 用户登出 |
| GET | `/api/auth/me` | 是 | 获取当前用户信息 |

---

### 3. 模型相关 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/models` | 是 | 获取支持的模型列表 |

**支持的模型：**
- `nano-banana-fast` - 快速图片生成模型（1K）
- `nano-banana` - 标准图片生成模型（1K）
- `nano-banana-pro` - 专业图片生成模型（1K/2K/4K）
- `nano-banana-pro-vt` - 专业图片生成模型（1K/2K/4K，支持 VT）
- `sora-2` - 视频生成模型

---

### 4. 提供商设置 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/settings/provider` | 是 | 获取提供商设置 |
| PUT | `/api/settings/provider` | 是 | 更新提供商设置 |

---

### 5. 管理员 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/admin/users` | 是 + 管理员 | 列出所有用户 |
| POST | `/api/admin/users` | 是 + 管理员 | 创建新用户 |
| DELETE | `/api/admin/users/:id` | 是 + 管理员 | 删除用户 |
| PATCH | `/api/admin/users/:id/status` | 是 + 管理员 | 更新用户状态（启用/禁用） |
| GET | `/api/admin/settings` | 是 + 管理员 | 获取系统设置 |
| PUT | `/api/admin/settings` | 是 + 管理员 | 更新系统设置 |

**系统设置参数：**
- `fileRetentionHours` - 文件保留时间（小时）
- `referenceHistoryLimit` - 参考历史限制
- `imageTimeoutSeconds` - 图片生成超时时间（秒）
- `videoTimeoutSeconds` - 视频生成超时时间（秒）

---

### 6. 生成相关 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/generations` | 是 | 列出生成记录 |
| GET | `/api/generations/:id` | 是 | 获取单个生成记录 |
| PATCH | `/api/generations/:id/favorite` | 是 | 切换收藏状态 |
| DELETE | `/api/generations/:id` | 是 | 删除生成记录 |
| POST | `/api/generate/image` | 是 | 生成图片 |
| POST | `/api/generate/video` | 是 | 生成视频 |

**查询参数（GET /api/generations）：**
- `type` - 生成类型（image/video）
- `onlyFavorites` - 仅显示收藏（1）
- `limit` - 返回数量限制（默认 50，最大 200）
- `offset` - 偏移量

---

### 7. 视频运行 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/video/runs` | 是 | 列出视频运行 |
| POST | `/api/video/runs` | 是 | 创建视频运行 |

---

### 8. 预设 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/presets` | 是 | 列出提示词预设 |
| POST | `/api/presets` | 是 | 创建提示词预设 |
| DELETE | `/api/presets/:id` | 是 | 删除提示词预设 |

---

### 9. 素材库 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/library` | 是 | 列出素材库项目 |
| POST | `/api/library` | 是 | 创建素材库项目 |
| DELETE | `/api/library/:id` | 是 | 删除素材库项目 |

**查询参数（GET /api/library）：**
- `kind` - 素材类型（role/scene）

---

### 10. 参考上传 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/reference-uploads` | 是 | 列出参考上传 |
| POST | `/api/reference-uploads` | 是 | 创建参考上传 |
| DELETE | `/api/reference-uploads/:id` | 是 | 删除参考上传 |

**查询参数（GET /api/reference-uploads）：**
- `limit` - 返回数量限制

---

### 11. 文件 API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/files/:id` | 是 | 获取文件（需认证） |
| GET | `/public/files/:id` | 否 | 获取公开文件（用于提供商获取参考图） |

**查询参数：**
- `token` - 认证令牌
- `download` - 是否下载（1）
- `thumb` - 是否返回缩略图（1）
- `filename` - 指定文件名

---

## 二、API 功能说明

### 2.1 认证功能

**登录 (POST /api/auth/login)**
- 请求体：`{ username: string, password: string }`
- 响应：`{ token: string, user: User }`
- 功能：用户登录验证，返回认证令牌和用户信息

**登出 (POST /api/auth/logout)**
- 功能：清除用户会话

**获取当前用户 (GET /api/auth/me)**
- 响应：`{ id: string, username: string, role: string, disabled: boolean }`
- 功能：获取当前登录用户的信息

---

### 2.2 模型功能

**获取模型列表 (GET /api/models)**
- 响应：`ModelInfo[]`
- 功能：获取所有支持的 AI 模型信息，包括模型类型、支持的分辨率、宽高比等

---

### 2.3 提供商设置功能

**获取提供商设置 (GET /api/settings/provider)**
- 响应：`{ providerHost: string, hasApiKey: boolean }`
- 功能：获取用户配置的 AI 服务提供商地址和 API 密钥状态

**更新提供商设置 (PUT /api/settings/provider)**
- 请求体：`{ providerHost: string, apiKey?: string }`
- 功能：更新用户的 AI 服务提供商配置

---

### 2.4 管理员功能

**列出用户 (GET /api/admin/users)**
- 响应：`AdminUserRow[]`
- 功能：管理员查看所有用户列表

**创建用户 (POST /api/admin/users)**
- 请求体：`{ username: string, password: string, role: 'admin' | 'user' }`
- 功能：管理员创建新用户

**删除用户 (DELETE /api/admin/users/:id)**
- 功能：管理员删除指定用户（不能删除自己）

**更新用户状态 (PATCH /api/admin/users/:id/status)**
- 请求体：`{ disabled: boolean }`
- 功能：管理员启用或禁用用户（不能禁用自己）

**获取系统设置 (GET /api/admin/settings)**
- 响应：`AdminSettings`
- 功能：管理员获取系统配置

**更新系统设置 (PUT /api/admin/settings)**
- 请求体：`Partial<AdminSettings>`
- 功能：管理员更新系统配置

---

### 2.5 生成功能

**列出生成记录 (GET /api/generations)**
- 响应：`{ items: Generation[], total: number }`
- 功能：分页查询用户的生成记录，支持按类型、收藏状态筛选

**获取生成记录 (GET /api/generations/:id)**
- 响应：`Generation`
- 功能：获取单个生成记录的详细信息

**切换收藏 (PATCH /api/generations/:id/favorite)**
- 响应：`Generation`
- 功能：切换生成记录的收藏状态

**删除生成记录 (DELETE /api/generations/:id)**
- 功能：删除指定的生成记录及其关联文件

**生成图片 (POST /api/generate/image)**
- 请求体：
  ```json
  {
    "prompt": string,
    "model": string,
    "imageSize": string,
    "aspectRatio": string,
    "batch": number,
    "referenceList": [
      { "type": "fileId" | "base64", "value": string }
    ]
  }
  ```
- 响应：`{ created: Generation[] }`
- 功能：生成图片，支持批量生成和参考图

**生成视频 (POST /api/generate/video)**
- 请求体：
  ```json
  {
    "prompt": string,
    "model": string,
    "aspectRatio": string,
    "duration": number,
    "videoSize": "small" | "large",
    "runId"?: string,
    "referenceFileIds"?: string[],
    "referenceBase64"?: string
  }
  ```
- 响应：`{ created: Generation, runId: string }`
- 功能：生成视频，支持参考图和视频运行

---

### 2.6 视频运行功能

**列出视频运行 (GET /api/video/runs)**
- 响应：`VideoRun[]`
- 功能：获取用户的视频运行列表

**创建视频运行 (POST /api/video/runs)**
- 请求体：`{ name: string }`
- 响应：`VideoRun`
- 功能：创建新的视频运行项目

---

### 2.7 预设功能

**列出预设 (GET /api/presets)**
- 响应：`PromptPreset[]`
- 功能：获取用户的提示词预设列表

**创建预设 (POST /api/presets)**
- 请求体：`{ name: string, prompt: string }`
- 响应：`PromptPreset`
- 功能：创建新的提示词预设

**删除预设 (DELETE /api/presets/:id)**
- 功能：删除指定的提示词预设

---

### 2.8 素材库功能

**列出素材库 (GET /api/library)**
- 响应：`LibraryItem[]`
- 功能：获取素材库项目，支持按类型筛选（role/scene）

**创建素材库项目 (POST /api/library)**
- 请求体：`FormData { kind: string, name: string, file: File }`
- 响应：`LibraryItem`
- 功能：上传文件到素材库

**删除素材库项目 (DELETE /api/library/:id)**
- 功能：删除指定的素材库项目

---

### 2.9 参考上传功能

**列出参考上传 (GET /api/reference-uploads)**
- 响应：`ReferenceUpload[]`
- 功能：获取参考图片上传列表

**创建参考上传 (POST /api/reference-uploads)**
- 请求体：`FormData { files: File[] }`
- 响应：`ReferenceUpload[]`
- 功能：批量上传参考图片

**删除参考上传 (DELETE /api/reference-uploads/:id)**
- 功能：删除指定的参考上传

---

### 2.10 文件功能

**获取文件 (GET /api/files/:id)**
- 功能：获取用户文件，支持下载、缩略图等选项

**获取公开文件 (GET /public/files/:id)**
- 功能：获取公开文件，用于 AI 服务提供商获取参考图

---

## 三、前端 React 调用方式

### 3.1 API 调用基础

前端使用 `services/api.ts` 中封装的 `api` 对象进行 API 调用。

**认证机制：**
- 使用 `localStorage` 存储 token（key: `nb_token`）
- 通过 `Authorization: Bearer {token}` header 传递认证信息
- 公开接口（如登录）不需要 token

**基础函数：**
```typescript
// 获取/设置 token
getAuthToken(): string | null
setAuthToken(token: string | null)
clearAuthToken()

// 构建 URL
url(path: string): string

// 构建 URL
buildFileUrl(fileId: string, opts?: { download?: boolean; filename?: string; thumb?: boolean }): string

// 核心请求函数
apiFetch<T>(path: string, options?: RequestInit & { raw?: boolean }): Promise<T>
```

---

### 3.2 具体调用示例

#### 3.2.1 认证相关

```typescript
// 登录
const { token, user } = await api.login(username, password);

// 登出
await api.logout();

// 获取当前用户
const user = await api.me();
```

---

#### 3.2.2 模型相关

```typescript
// 获取模型列表
const models = await api.getModels();
```

---

#### 3.2.3 提供商设置

```typescript
// 获取提供商设置
const settings = await api.getProviderSettings();

// 更新提供商设置
const updated = await api.updateProviderSettings({
  providerHost: 'https://api.example.com',
  apiKey: 'your-api-key'
});
```

---

#### 3.2.4 生成相关

```typescript
// 列出生成记录
const { items, total } = await api.listGenerations({
  type: 'image',
  onlyFavorites: true,
  limit: 50,
  offset: 0
});

// 获取单个生成记录
const gen = await api.getGeneration(id);

// 切换收藏
const updated = await api.toggleFavorite(id);

// 删除生成记录
await api.deleteGeneration(id);

// 生成图片
const result = await api.generateImages({
  prompt: 'A beautiful sunset',
  model: 'nano-banana-pro',
  imageSize: '1024x1024',
  aspectRatio: '1:1',
  batch: 4,
  orderedReferences: [
    { fileId: 'file-id-1' },
    { file: uploadedFile }
  ]
});

// 生成视频
const result = await api.generateVideo({
  prompt: 'A person walking',
  model: 'sora-2',
  aspectRatio: '16:9',
  duration: 5,
  videoSize: 'large',
  runId: 'run-id',
  referenceFileIds: ['file-id-1'],
  referenceUpload: uploadedFile
});
```

---

#### 3.2.5 预设相关

```typescript
// 列出预设
const presets = await api.listPresets();

// 创建预设
const preset = await api.createPreset({
  name: 'My Preset',
  prompt: 'A beautiful landscape'
});

// 删除预设
await api.deletePreset(id);
```

---

#### 3.2.6 素材库相关

```typescript
// 列出素材库
const items = await api.listLibrary('role');

// 上传素材库项目
const item = await api.uploadLibraryItem({
  kind: 'role',
  name: 'Character 1',
  file: uploadedFile
});

// 删除素材库项目
await api.deleteLibraryItem(id);
```

---

#### 3.2.7 参考上传相关

```typescript
// 列出参考上传
const uploads = await api.listReferenceUploads({ limit: 50 });

// 上传参考图片
const uploaded = await api.uploadReferenceUploads([file1, file2, file3]);

// 删除参考上传
await api.deleteReferenceUpload(id);
```

---

#### 3.2.8 视频运行相关

```typescript
// 列出视频运行
const runs = await api.listVideoRuns();

// 创建视频运行
const run = await api.createVideoRun({ name: 'My Video Run' });
```

---

#### 3.2.9 管理员相关

```typescript
// 列出用户
const users = await api.adminListUsers();

// 创建用户
const user = await api.adminCreateUser({
  username: 'newuser',
  password: 'password123',
  role: 'user'
});

// 删除用户
await api.adminDeleteUser(userId);

// 更新用户状态
const updated = await api.adminUpdateUserStatus(userId, true);

// 获取系统设置
const settings = await api.adminGetSettings();

// 更新系统设置
const updated = await api.adminUpdateSettings({
  fileRetentionHours: 24,
  referenceHistoryLimit: 100
});
```

---

### 3.3 文件 URL 构建

```typescript
// 构建文件 URL
const url = buildFileUrl(fileId, {
  download: true,
  filename: 'image.png',
  thumb: false
});
```

---

### 3.4 错误处理

所有 API 调用都会抛出错误，前端需要使用 try-catch 处理：

```typescript
try {
  const result = await api.generateImages({ ... });
} catch (error) {
  console.error('生成失败:', error.message);
}
```

---

## 四、数据模型

### 4.1 后端数据模型

**User**
```go
type User struct {
  ID           string
  Username     string
  Role         string
  PasswordHash string
  Disabled     bool
  CreatedAt    int64
}
```

**Generation**
```go
type Generation struct {
  ID                string
  UserID            string
  Type              string
  Prompt            string
  Model             string
  Status            string
  Progress          *float64
  StartedAt         *int64
  ElapsedSeconds    *int64
  Error             *string
  ReferenceFileIDs  []string
  ImageSize         *string
  AspectRatio       *string
  Favorite          bool
  OutputFileID      *string
  Duration          *int
  VideoSize         *string
  RunID             *string
  NodePosition      *int
  CreatedAt         int64
  UpdatedAt         int64
}
```

---

### 4.2 前端数据模型

**Generation**
```typescript
interface Generation {
  id: string;
  type: 'image' | 'video';
  prompt: string;
  model: string;
  status: 'queued' | 'running' | 'succeeded' | 'failed';
  createdAt: number;
  updatedAt: number;
  progress?: number;
  startedAt?: number;
  elapsedSeconds?: number;
  error?: string;
  favorite?: boolean;
  imageSize?: string;
  aspectRatio?: string;
  duration?: number;
  videoSize?: 'small' | 'large';
  outputFile?: StoredFile | null;
  referenceFileIds?: string[];
  runId?: string;
  nodePosition?: number;
}
```

---

## 五、项目结构

### 5.1 后端结构

```
nano-backend/
├── internal/
│   ├── config/          # 配置管理
│   ├── crypto/          # 加密相关
│   ├── database/        # 数据库操作
│   ├── fileutil/        # 文件工具
│   ├── grsai/           # AI 服务集成
│   ├── handlers/        # HTTP 处理器
│   ├── jobs/            # 后台任务
│   ├── middleware/      # 中间件
│   └── models/          # 数据模型
├── data/                # 数据库文件
├── main.go              # 入口文件
└── go.mod               # Go 模块定义
```

---

### 5.2 前端结构

```
nano-frontend/
├── components/          # React 组件
│   ├── ErrorBoundary.tsx
│   ├── GenerationCard.tsx
│   ├── HistoryPickerModal.tsx
│   ├── ImageGallery.tsx
│   ├── ImagePreviewModal.tsx
│   ├── LibraryPickerModal.tsx
│   ├── Modal.tsx
│   ├── PromptPresetsModal.tsx
│   ├── ProviderSettingsModal.tsx
│   ├── Spinner.tsx
│   └── VideoNodeCard.tsx
├── pages/               # 页面组件
│   ├── AdminPage.tsx
│   ├── ImagePage.tsx
│   ├── LibraryPage.tsx
│   ├── LoginPage.tsx
│   ├── SlicerPage.tsx
│   └── VideoPage.tsx
├── services/            # API 服务
│   └── api.ts
├── App.tsx              # 主应用组件
├── types.ts             # TypeScript 类型定义
└── package.json         # 依赖配置
```

---

## 六、总结

本项目采用前后端分离架构，后端使用 Go + Fiber 提供 RESTful API，前端使用 React + TypeScript 进行用户界面开发。API 设计清晰，功能完整，支持图片和视频的 AI 生成、用户管理、素材库管理等核心功能。前端通过封装的 API 服务层与后端进行通信，使用 token 进行身份认证，支持文件上传、下载等操作。
