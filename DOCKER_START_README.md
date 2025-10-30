# NOFX Docker 启动脚本使用说明

`docker-start.sh` 是一个用于构建和启动 NOFX 前后端 Docker 容器的便捷脚本。

## 📋 目录

- [快速开始](#快速开始)
- [命令说明](#命令说明)
- [使用示例](#使用示例)
- [环境变量](#环境变量)
- [常见问题](#常见问题)

## 🚀 快速开始

### 1. 准备工作

确保已安装 Docker：
```bash
docker --version
```

确保配置文件存在：
```bash
# 如果不存在，从模板复制
cp config.json.example config.json
# 编辑配置文件，填入你的 API 密钥等信息
nano config.json
```

### 2. 构建并启动

最简单的方式：
```bash
# 构建所有镜像并启动所有容器
./docker-start.sh build && ./docker-start.sh start
```

或者分步执行：
```bash
# 步骤1: 构建镜像
./docker-start.sh build

# 步骤2: 启动容器
./docker-start.sh start
```

### 3. 访问服务

启动成功后，可以访问：
- **前端界面**: http://localhost:3000
- **后端 API**: http://localhost:8080
- **健康检查**: 
  - 后端: http://localhost:8080/health
  - 前端: http://localhost:3000/health

## 📖 命令说明

### 构建镜像

```bash
./docker-start.sh build [backend|frontend|all]
```

**说明**:
- `backend`: 只构建后端镜像
- `frontend`: 只构建前端镜像
- `all`: 构建所有镜像（默认）

**示例**:
```bash
./docker-start.sh build              # 构建所有镜像
./docker-start.sh build backend       # 只构建后端
./docker-start.sh build frontend      # 只构建前端
```

### 启动容器

```bash
./docker-start.sh start [backend|frontend|all] [--config PATH]
```

**说明**:
- `backend`: 只启动后端容器
- `frontend`: 只启动前端容器（需要后端已运行）
- `all`: 启动所有容器（默认）
- `--config PATH`: 指定配置文件路径（可选）

**示例**:
```bash
./docker-start.sh start                                    # 启动所有容器
./docker-start.sh start backend                            # 只启动后端
./docker-start.sh start --config /path/to/config.json      # 使用自定义配置
./docker-start.sh start backend --config ./prod-config.json # 启动后端并使用自定义配置
```

### 停止容器

```bash
./docker-start.sh stop [backend|frontend|all]
```

**说明**:
- `backend`: 只停止后端容器
- `frontend`: 只停止前端容器
- `all`: 停止所有容器（默认）

**示例**:
```bash
./docker-start.sh stop              # 停止所有容器
./docker-start.sh stop backend       # 只停止后端
./docker-start.sh stop frontend      # 只停止前端
```

### 重启容器

```bash
./docker-start.sh restart [backend|frontend|all] [--config PATH]
```

**说明**: 先停止容器，然后重新启动

**示例**:
```bash
./docker-start.sh restart           # 重启所有容器
./docker-start.sh restart backend   # 重启后端
```

### 查看日志

```bash
./docker-start.sh logs [backend|frontend|all]
```

**说明**:
- `backend`: 只查看后端日志
- `frontend`: 只查看前端日志
- `all`: 查看所有日志（默认）

**示例**:
```bash
./docker-start.sh logs              # 查看所有日志
./docker-start.sh logs backend       # 只查看后端日志
./docker-start.sh logs frontend      # 只查看前端日志
```

**提示**: 按 `Ctrl+C` 退出日志查看

### 查看状态

```bash
./docker-start.sh status
```

**说明**: 显示容器状态、镜像信息、网络信息和健康检查结果

**示例**:
```bash
./docker-start.sh status
```

### 删除容器

```bash
./docker-start.sh remove [backend|frontend|all]
```

**说明**: 删除容器（需要确认）

**示例**:
```bash
./docker-start.sh remove            # 删除所有容器
./docker-start.sh remove backend     # 只删除后端容器
```

### 清理镜像

```bash
./docker-start.sh clean [backend|frontend|all]
```

**说明**: 删除 Docker 镜像（需要确认）

**示例**:
```bash
./docker-start.sh clean             # 删除所有镜像
./docker-start.sh clean backend     # 只删除后端镜像
```

### 查看帮助

```bash
./docker-start.sh help
# 或
./docker-start.sh --help
# 或
./docker-start.sh -h
```

## 📝 使用示例

### 场景1: 首次部署

```bash
# 1. 检查配置文件
cat config.json

# 2. 构建所有镜像
./docker-start.sh build

# 3. 启动所有服务
./docker-start.sh start

# 4. 查看启动状态
./docker-start.sh status

# 5. 查看日志（可选）
./docker-start.sh logs
```

### 场景2: 使用自定义配置文件

```bash
# 1. 准备生产环境配置文件
cp config.json prod-config.json
# 编辑 prod-config.json...

# 2. 使用自定义配置启动
./docker-start.sh start --config ./prod-config.json
```

### 场景3: 只更新前端

```bash
# 1. 停止前端容器
./docker-start.sh stop frontend

# 2. 重新构建前端镜像
./docker-start.sh build frontend

# 3. 删除旧的前端容器
./docker-start.sh remove frontend

# 4. 启动新的前端容器
./docker-start.sh start frontend
```

### 场景4: 只更新后端

```bash
# 1. 停止所有容器
./docker-start.sh stop

# 2. 重新构建后端镜像
./docker-start.sh build backend

# 3. 删除旧容器
./docker-start.sh remove backend

# 4. 启动所有服务
./docker-start.sh start
```

### 场景5: 查看后端日志进行调试

```bash
# 实时查看后端日志
./docker-start.sh logs backend
```

### 场景6: 完全重启服务

```bash
# 1. 停止所有容器
./docker-start.sh stop

# 2. 删除所有容器
./docker-start.sh remove

# 3. 重新构建镜像（如果需要）
./docker-start.sh build

# 4. 启动所有服务
./docker-start.sh start
```

## 🔧 环境变量

可以通过环境变量自定义配置：

```bash
# 镜像配置
export NOFX_BACKEND_IMAGE_NAME="my-nofx-backend"
export NOFX_FRONTEND_IMAGE_NAME="my-nofx-frontend"
export NOFX_IMAGE_TAG="v1.0.0"

# 容器配置
export NOFX_BACKEND_CONTAINER_NAME="my-backend"
export NOFX_FRONTEND_CONTAINER_NAME="my-frontend"
export NOFX_NETWORK_NAME="my-network"

# 端口配置
export NOFX_BACKEND_PORT="9090"
export NOFX_FRONTEND_PORT="4000"

# 配置文件路径
export NOFX_CONFIG_FILE="./prod-config.json"

# 决策日志目录
export NOFX_DECISION_LOGS="./logs"

# 使用时
./docker-start.sh start
```

### 环境变量列表

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `NOFX_BACKEND_IMAGE_NAME` | 后端镜像名称 | `nofx-backend` |
| `NOFX_FRONTEND_IMAGE_NAME` | 前端镜像名称 | `nofx-frontend` |
| `NOFX_IMAGE_TAG` | 镜像标签 | `latest` |
| `NOFX_BACKEND_CONTAINER_NAME` | 后端容器名称 | `nofx-trading` |
| `NOFX_FRONTEND_CONTAINER_NAME` | 前端容器名称 | `nofx-frontend` |
| `NOFX_NETWORK_NAME` | Docker 网络名称 | `nofx-network` |
| `NOFX_CONFIG_FILE` | 默认配置文件路径 | `./config.json` |
| `NOFX_BACKEND_PORT` | 后端 API 端口 | `8080` |
| `NOFX_FRONTEND_PORT` | 前端端口 | `3000` |
| `NOFX_DECISION_LOGS` | 决策日志目录 | `./decision_logs` |

## ❓ 常见问题

### Q1: 如何查看容器是否正在运行？

```bash
./docker-start.sh status
```

或者使用 Docker 命令：
```bash
docker ps
```

### Q2: 容器启动失败怎么办？

1. 查看日志：
```bash
./docker-start.sh logs backend
```

2. 检查配置文件是否存在：
```bash
ls -la config.json
```

3. 检查端口是否被占用：
```bash
lsof -i :8080  # 检查后端端口
lsof -i :3000  # 检查前端端口
```

### Q3: 如何修改配置文件？

1. 停止容器：
```bash
./docker-start.sh stop
```

2. 编辑配置文件：
```bash
nano config.json
```

3. 重启容器：
```bash
./docker-start.sh start
```

或者使用外部配置文件：
```bash
./docker-start.sh start --config /path/to/new-config.json
```

### Q4: 如何清理所有数据重新开始？

```bash
# 1. 停止所有容器
./docker-start.sh stop

# 2. 删除所有容器
./docker-start.sh remove

# 3. 清理所有镜像（可选）
./docker-start.sh clean

# 4. 清理网络（手动）
docker network rm nofx-network

# 5. 重新构建和启动
./docker-start.sh build && ./docker-start.sh start
```

### Q5: 前端无法访问后端 API？

1. 确保后端容器正在运行：
```bash
./docker-start.sh status
```

2. 检查后端健康状态：
```bash
curl http://localhost:8080/health
```

3. 检查前端日志：
```bash
./docker-start.sh logs frontend
```

4. 确保网络已创建：
```bash
docker network ls | grep nofx-network
```

### Q6: 如何备份配置文件？

```bash
# 备份当前配置
cp config.json config.json.backup

# 使用备份配置启动
./docker-start.sh start --config config.json.backup
```

### Q7: 如何查看镜像大小？

```bash
./docker-start.sh status
```

或者：
```bash
docker images | grep nofx
```

### Q8: 如何在生产环境使用？

1. 创建生产环境配置文件：
```bash
cp config.json config.prod.json
# 编辑 config.prod.json，填入生产环境配置
```

2. 使用环境变量和自定义配置：
```bash
NOFX_CONFIG_FILE=./config.prod.json \
NOFX_BACKEND_PORT=8080 \
NOFX_FRONTEND_PORT=3000 \
./docker-start.sh start
```

3. 设置自动重启（容器已配置 `--restart unless-stopped`）

## 📌 注意事项

1. **配置文件路径**: 使用 `--config` 时，请使用绝对路径或相对于当前目录的路径

2. **端口冲突**: 如果端口被占用，请使用环境变量修改端口：
   ```bash
   NOFX_BACKEND_PORT=9090 NOFX_FRONTEND_PORT=4000 ./docker-start.sh start
   ```

3. **网络**: 脚本会自动创建 Docker 网络，如果网络已存在会跳过创建

4. **依赖关系**: 前端容器依赖后端容器，如果只启动前端，需要先确保后端运行

5. **数据持久化**: 
   - 配置文件通过 volume 挂载，修改配置后需要重启容器
   - 决策日志存储在 `./decision_logs` 目录（可通过环境变量修改）

6. **构建时间**: 首次构建可能需要较长时间，请耐心等待

## 🔗 相关文档

- [Docker 部署文档](./DOCKER_DEPLOY.md)
- [配置文件说明](./config.json.example)
- [常见问题](./常见问题.md)

## 📞 获取帮助

如果遇到问题，可以：
1. 查看日志：`./docker-start.sh logs`
2. 查看状态：`./docker-start.sh status`
3. 查看帮助：`./docker-start.sh help`

