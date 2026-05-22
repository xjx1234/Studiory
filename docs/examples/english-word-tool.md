# 英文单词工具 API 设计（示例）

> **非脚手架内置**：仅供参考如何设计用户端/管理端 API。实现时请按 [architecture.md](../architecture.md) 新建业务模块。

> 本文件只定义结构和边界，不关心具体业务实现细节。

## 模块划分

- **用户端 API（User API）**
  - 面向学生/普通用户。
  - 提供练习、查看结果、错题回顾等能力。
- **管理端 API（Admin API）**
  - 面向老师/运营/内容维护人员。
  - 提供单词库管理、练习配置管理、统计报表等能力。

统一前缀建议：

- 用户端：`/api/v1/user/english-word/...`
- 管理端：`/api/v1/admin/english-word/...`

## 用户端 API（User）

### 1. 获取可用练习模式

- **Method**: `GET`
- **Path**: `/api/v1/user/english-word/modes`
- **Query**:
  - `grade` (可选，string)：年级，如 `G3`、`G4` 等。
  - `level` (可选, string)：难度等级，如 `easy` / `normal` / `hard`。
- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": [
    {
      "code": "dictation",
      "name": "听写模式",
      "description": "听读单词后拼写单词",
      "enabled": true
    },
    {
      "code": "spelling",
      "name": "拼写模式",
      "description": "给出中文或英文释义，输入英文单词",
      "enabled": true
    },
    {
      "code": "meaning",
      "name": "词义模式",
      "description": "给出英文单词，选择或输入中文释义",
      "enabled": true
    }
  ]
}
```

### 2. 创建一次练习会话

- **Method**: `POST`
- **Path**: `/api/v1/user/english-word/sessions`
- **Body**:

```json
{
  "mode": "dictation",
  "grade": "G3",
  "level": "normal",
  "wordSetId": "primary_g3_unit1",  // 可选，指定一个单词集
  "questionCount": 10               // 可选，默认 10
}
```

- **Response** `201 Created`:

```json
{
  "status": "ok",
  "data": {
    "sessionId": "sess_123456",
    "mode": "dictation",
    "questions": [
      {
        "id": "q1",
        "wordId": "w_apple",
        "promptType": "audio",        // audio / text / image
        "prompt": "https://.../audio/apple.mp3",
        "extra": {
          "hint": "水果",
          "phonetic": "[ˈæpl]"       // 可选
        }
      }
    ]
  }
}
```

> 说明：如果未来由 Python 服务来生成题目，可以在内部通过 RPC 调用进行替换，但保持返回结构不变。

### 3. 提交练习结果

- **Method**: `POST`
- **Path**: `/api/v1/user/english-word/sessions/{sessionId}/submit`
- **Body**:

```json
{
  "answers": [
    {
      "questionId": "q1",
      "wordId": "w_apple",
      "userAnswer": "applle",
      "timeUsedMs": 3500
    }
  ]
}
```

- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": {
    "sessionId": "sess_123456",
    "score": 86,
    "correctCount": 8,
    "wrongCount": 2,
    "details": [
      {
        "questionId": "q1",
        "wordId": "w_apple",
        "userAnswer": "applle",
        "correctAnswer": "apple",
        "isCorrect": false,
        "explanation": "多打了一个 l",
        "wordInfo": {
          "text": "apple",
          "meaning": "苹果",
          "phonetic": "[ˈæpl]"
        }
      }
    ]
  }
}
```

### 4. 获取某次练习结果（回顾）

- **Method**: `GET`
- **Path**: `/api/v1/user/english-word/sessions/{sessionId}`
- **Response** `200 OK`:

与「提交练习结果」的 `data` 结构基本一致，可视作查询版。

## 管理端 API（Admin）

### 1. 管理端登录（预留）

- **Method**: `POST`
- **Path**: `/api/v1/admin/auth/login`
- **Body**:

```json
{
  "username": "admin",
  "password": "******"
}
```

- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": {
    "token": "jwt_token_or_other"
  }
}
```

> 说明：具体认证方式（JWT、Session、单点登录）可后续确定。

### 2. 单词集列表

- **Method**: `GET`
- **Path**: `/api/v1/admin/english-word/word-sets`
- **Query（可选）**:
  - `page`, `pageSize`
  - `grade`
  - `keyword`

- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": {
    "items": [
      {
        "id": "primary_g3_unit1",
        "name": "三年级上册 Unit 1",
        "grade": "G3",
        "wordCount": 120,
        "enabled": true
      }
    ],
    "total": 1
  }
}
```

### 3. 创建/编辑单词集

- **Method**: `POST`
- **Path**: `/api/v1/admin/english-word/word-sets`

- **Body**:

```json
{
  "id": "primary_g3_unit1",       // 可选，缺省表示创建，新值表示编辑
  "name": "三年级上册 Unit 1",
  "grade": "G3",
  "enabled": true
}
```

- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": {
    "id": "primary_g3_unit1"
  }
}
```

### 4. 单词列表（某个单词集下）

- **Method**: `GET`
- **Path**: `/api/v1/admin/english-word/word-sets/{wordSetId}/words`
- **Query（可选）**:
  - `page`, `pageSize`
  - `keyword`

- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": {
    "items": [
      {
        "id": "w_apple",
        "text": "apple",
        "meaning": "苹果",
        "phonetic": "[ˈæpl]",
        "audioUrl": "https://.../apple.mp3",
        "tags": ["水果", "常见词"]
      }
    ],
    "total": 1
  }
}
```

### 5. 创建/编辑单词

- **Method**: `POST`
- **Path**: `/api/v1/admin/english-word/word-sets/{wordSetId}/words`

- **Body**:

```json
{
  "id": "w_apple",          // 可选，缺省表示创建，新值表示编辑
  "text": "apple",
  "meaning": "苹果",
  "phonetic": "[ˈæpl]",
  "audioUrl": "https://.../apple.mp3",
  "tags": ["水果", "常见词"]
}
```

- **Response** `200 OK`:

```json
{
  "status": "ok",
  "data": {
    "id": "w_apple"
  }
}
```

## 与内部服务的关系（Golang / Node / Python）

- 对前端和第三方调用方而言，只需要关心上述 HTTP API。
- 在 **Golang 后端内部**：
  - 用户端练习题目的生成，可以：
    - 先用 Go 实现简单「从单词集中随机抽题」；
    - 未来再切换为调用 **Python 服务** 的智能出题接口。
  - 管理端的增删改查，可直接由 Go 连接数据库完成。
  - 如果后续需要 **Node BFF** 或 WebSocket 能力，可以由 Node 再去调用 Golang 主后端或订阅事件。

> 下一步：在 `backend/internal` 中按照「user/admin/domain」的结构拆分代码骨架，并在 `backend/main.go` 中为上述 API 预留路由占位。

