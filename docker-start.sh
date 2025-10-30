#!/bin/bash

# ═══════════════════════════════════════════════════════════════
# NOFX Docker 打包和启动脚本
# 支持外部配置文件路径
# Usage: ./docker-start.sh [command] [options]
# ═══════════════════════════════════════════════════════════════

set -e

# ------------------------------------------------------------------------
# Color Definitions
# ------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# ------------------------------------------------------------------------
# Configuration
# ------------------------------------------------------------------------
BACKEND_IMAGE_NAME="${NOFX_BACKEND_IMAGE_NAME:-nofx-backend}"
FRONTEND_IMAGE_NAME="${NOFX_FRONTEND_IMAGE_NAME:-nofx-frontend}"
IMAGE_TAG="${NOFX_IMAGE_TAG:-latest}"
BACKEND_CONTAINER_NAME="${NOFX_BACKEND_CONTAINER_NAME:-nofx-trading}"
FRONTEND_CONTAINER_NAME="${NOFX_FRONTEND_CONTAINER_NAME:-nofx-frontend}"
NETWORK_NAME="${NOFX_NETWORK_NAME:-nofx-network}"
DEFAULT_CONFIG_FILE="${NOFX_CONFIG_FILE:-./config.json}"
CONTAINER_CONFIG_PATH="/app/config.json"
BACKEND_PORT="${NOFX_BACKEND_PORT:-8080}"
FRONTEND_PORT="${NOFX_FRONTEND_PORT:-3000}"
DECISION_LOGS_DIR="${NOFX_DECISION_LOGS:-./decision_logs}"

# ------------------------------------------------------------------------
# Utility Functions: Colored Output
# ------------------------------------------------------------------------
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

# ------------------------------------------------------------------------
# Validation: Docker Installation
# ------------------------------------------------------------------------
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker 未安装！请先安装 Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi
    print_success "Docker 已安装: $(docker --version)"
}

# ------------------------------------------------------------------------
# Validation: Configuration File
# ------------------------------------------------------------------------
check_config() {
    local config_file="${1:-$DEFAULT_CONFIG_FILE}"
    
    if [ ! -f "$config_file" ]; then
        print_error "配置文件不存在: $config_file"
        print_info "请创建配置文件，或使用 --config 参数指定其他路径"
        print_info "示例: ./docker-start.sh build --config /path/to/config.json"
        exit 1
    fi
    print_success "配置文件存在: $config_file"
}

# ------------------------------------------------------------------------
# Network: Create Docker Network
# ------------------------------------------------------------------------
create_network() {
    if ! docker network ls --format '{{.Name}}' | grep -q "^${NETWORK_NAME}$"; then
        print_info "创建 Docker 网络: ${NETWORK_NAME}"
        docker network create "${NETWORK_NAME}" > /dev/null 2>&1
        print_success "网络创建成功: ${NETWORK_NAME}"
    else
        print_info "网络已存在: ${NETWORK_NAME}"
    fi
}

# ------------------------------------------------------------------------
# Build: Docker Images
# ------------------------------------------------------------------------
build_backend_image() {
    local dockerfile_path="./docker/Dockerfile.backend"
    
    if [ ! -f "$dockerfile_path" ]; then
        print_error "Dockerfile 不存在: $dockerfile_path"
        exit 1
    fi
    
    print_info "构建后端镜像: ${BACKEND_IMAGE_NAME}:${IMAGE_TAG}"
    print_info "Dockerfile: $dockerfile_path"
    
    docker build \
        -f "$dockerfile_path" \
        -t "${BACKEND_IMAGE_NAME}:${IMAGE_TAG}" \
        --build-arg GO_VERSION="${GO_VERSION:-1.25-alpine}" \
        --build-arg ALPINE_VERSION="${ALPINE_VERSION:-latest}" \
        --build-arg TA_LIB_VERSION="${TA_LIB_VERSION:-0.4.0}" \
        .
    
    if [ $? -eq 0 ]; then
        print_success "后端镜像构建成功: ${BACKEND_IMAGE_NAME}:${IMAGE_TAG}"
    else
        print_error "后端镜像构建失败"
        exit 1
    fi
}

build_frontend_image() {
    local dockerfile_path="./docker/Dockerfile.frontend"
    
    if [ ! -f "$dockerfile_path" ]; then
        print_error "Dockerfile 不存在: $dockerfile_path"
        exit 1
    fi
    
    print_info "构建前端镜像: ${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}"
    print_info "Dockerfile: $dockerfile_path"
    
    docker build \
        -f "$dockerfile_path" \
        -t "${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}" \
        --build-arg NODE_VERSION="${NODE_VERSION:-20-alpine}" \
        --build-arg NGINX_VERSION="${NGINX_VERSION:-alpine}" \
        .
    
    if [ $? -eq 0 ]; then
        print_success "前端镜像构建成功: ${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}"
    else
        print_error "前端镜像构建失败"
        exit 1
    fi
}

build_images() {
    print_header "构建 Docker 镜像"
    
    local build_type="${1:-all}"
    
    case "$build_type" in
        backend)
            build_backend_image
            ;;
        frontend)
            build_frontend_image
            ;;
        all)
            print_info "开始构建所有镜像（这可能需要几分钟）..."
            build_backend_image
            echo ""
            build_frontend_image
            ;;
        *)
            print_error "未知构建类型: $build_type"
            exit 1
            ;;
    esac
}

# ------------------------------------------------------------------------
# Service Management: Start Containers
# ------------------------------------------------------------------------
start_backend_container() {
    local config_file="${1:-$DEFAULT_CONFIG_FILE}"
    local container_config_path="${2:-$CONTAINER_CONFIG_PATH}"
    
    # 检查配置文件
    check_config "$config_file"
    
    # 检查镜像是否存在
    if ! docker images | grep -q "^${BACKEND_IMAGE_NAME}\s\+${IMAGE_TAG}"; then
        print_warning "后端镜像不存在，开始构建..."
        build_backend_image
    fi
    
    # 检查容器是否已运行
    if docker ps --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
        print_warning "后端容器已在运行: ${BACKEND_CONTAINER_NAME}"
        return 1
    fi
    
    # 创建 decision_logs 目录（如果不存在）
    mkdir -p "$DECISION_LOGS_DIR"
    
    # 启动后端容器
    print_info "启动后端容器: ${BACKEND_CONTAINER_NAME}"
    print_info "配置文件: ${config_file} -> ${container_config_path}"
    print_info "API 端口: ${BACKEND_PORT}"
    
    # 构建 docker run 命令数组
    local docker_args=(
        -d
        --name "${BACKEND_CONTAINER_NAME}"
        --network "${NETWORK_NAME}"
        --network-alias nofx
        --restart unless-stopped
        -p "${BACKEND_PORT}:8080"
        -v "$(realpath "$config_file"):${container_config_path}:ro"
        -v "$(realpath "$DECISION_LOGS_DIR"):/app/decision_logs"
        -v /etc/localtime:/etc/localtime:ro
        -e "TZ=${TZ:-Asia/Shanghai}"
    )
    
    # 添加镜像名称
    docker_args+=("${BACKEND_IMAGE_NAME}:${IMAGE_TAG}")
    
    # 如果使用自定义配置路径，添加作为参数
    if [ "$config_file" != "$DEFAULT_CONFIG_FILE" ] || [ "$container_config_path" != "$CONTAINER_CONFIG_PATH" ]; then
        docker_args+=("${container_config_path}")
    fi
    
    docker run "${docker_args[@]}"
    
    if [ $? -eq 0 ]; then
        print_success "后端容器启动成功"
        return 0
    else
        print_error "后端容器启动失败"
        return 1
    fi
}

start_frontend_container() {
    # 检查后端容器是否运行
    if ! docker ps --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
        print_error "后端容器未运行，请先启动后端"
        return 1
    fi
    
    # 检查镜像是否存在
    if ! docker images | grep -q "^${FRONTEND_IMAGE_NAME}\s\+${IMAGE_TAG}"; then
        print_warning "前端镜像不存在，开始构建..."
        build_frontend_image
    fi
    
    # 检查容器是否已运行
    if docker ps --format '{{.Names}}' | grep -q "^${FRONTEND_CONTAINER_NAME}$"; then
        print_warning "前端容器已在运行: ${FRONTEND_CONTAINER_NAME}"
        return 1
    fi
    
    # 启动前端容器
    print_info "启动前端容器: ${FRONTEND_CONTAINER_NAME}"
    print_info "前端端口: ${FRONTEND_PORT}"
    
    docker run -d \
        --name "${FRONTEND_CONTAINER_NAME}" \
        --network "${NETWORK_NAME}" \
        --restart unless-stopped \
        -p "${FRONTEND_PORT}:80" \
        "${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}"
    
    if [ $? -eq 0 ]; then
        print_success "前端容器启动成功"
        return 0
    else
        print_error "前端容器启动失败"
        return 1
    fi
}

start_containers() {
    print_header "启动 Docker 容器"
    
    local service_type="${1:-all}"
    local config_file="${2:-$DEFAULT_CONFIG_FILE}"
    local container_config_path="${3:-$CONTAINER_CONFIG_PATH}"
    
    # 创建网络
    create_network
    
    case "$service_type" in
        backend)
            start_backend_container "$config_file" "$container_config_path"
            ;;
        frontend)
            start_frontend_container
            ;;
        all)
            if start_backend_container "$config_file" "$container_config_path"; then
                echo ""
                sleep 2  # 等待后端启动
                start_frontend_container
            fi
            ;;
        *)
            print_error "未知服务类型: $service_type"
            exit 1
            ;;
    esac
    
    if [ $? -eq 0 ]; then
        echo ""
        print_success "所有容器启动成功！"
        echo ""
        print_info "Web 界面: http://localhost:${FRONTEND_PORT}"
        print_info "API 端点: http://localhost:${BACKEND_PORT}"
        print_info "后端健康检查: http://localhost:${BACKEND_PORT}/health"
        print_info "前端健康检查: http://localhost:${FRONTEND_PORT}/health"
        echo ""
        print_info "查看日志: ./docker-start.sh logs [backend|frontend]"
        print_info "停止容器: ./docker-start.sh stop"
        print_info "查看状态: ./docker-start.sh status"
    fi
}

# ------------------------------------------------------------------------
# Service Management: Stop Containers
# ------------------------------------------------------------------------
stop_container() {
    print_header "停止 Docker 容器"
    
    local service_type="${1:-all}"
    
    case "$service_type" in
        backend)
            if docker ps --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
                print_info "正在停止后端容器: ${BACKEND_CONTAINER_NAME}"
                docker stop "${BACKEND_CONTAINER_NAME}"
                print_success "后端容器已停止"
            else
                print_warning "后端容器未运行: ${BACKEND_CONTAINER_NAME}"
            fi
            ;;
        frontend)
            if docker ps --format '{{.Names}}' | grep -q "^${FRONTEND_CONTAINER_NAME}$"; then
                print_info "正在停止前端容器: ${FRONTEND_CONTAINER_NAME}"
                docker stop "${FRONTEND_CONTAINER_NAME}"
                print_success "前端容器已停止"
            else
                print_warning "前端容器未运行: ${FRONTEND_CONTAINER_NAME}"
            fi
            ;;
        all)
            if docker ps --format '{{.Names}}' | grep -q "^${FRONTEND_CONTAINER_NAME}$"; then
                print_info "正在停止前端容器: ${FRONTEND_CONTAINER_NAME}"
                docker stop "${FRONTEND_CONTAINER_NAME}"
            fi
            if docker ps --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
                print_info "正在停止后端容器: ${BACKEND_CONTAINER_NAME}"
                docker stop "${BACKEND_CONTAINER_NAME}"
            fi
            print_success "所有容器已停止"
            ;;
        *)
            print_error "未知服务类型: $service_type"
            exit 1
            ;;
    esac
}

# ------------------------------------------------------------------------
# Service Management: Remove Containers
# ------------------------------------------------------------------------
remove_container() {
    print_header "删除 Docker 容器"
    
    local service_type="${1:-all}"
    
    case "$service_type" in
        backend)
            if docker ps -a --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
                print_warning "这将删除后端容器: ${BACKEND_CONTAINER_NAME}"
                read -p "确认删除？(yes/no): " confirm
                if [ "$confirm" == "yes" ]; then
                    docker rm -f "${BACKEND_CONTAINER_NAME}" 2>/dev/null || true
                    print_success "后端容器已删除"
                else
                    print_info "已取消"
                fi
            else
                print_warning "后端容器不存在: ${BACKEND_CONTAINER_NAME}"
            fi
            ;;
        frontend)
            if docker ps -a --format '{{.Names}}' | grep -q "^${FRONTEND_CONTAINER_NAME}$"; then
                print_warning "这将删除前端容器: ${FRONTEND_CONTAINER_NAME}"
                read -p "确认删除？(yes/no): " confirm
                if [ "$confirm" == "yes" ]; then
                    docker rm -f "${FRONTEND_CONTAINER_NAME}" 2>/dev/null || true
                    print_success "前端容器已删除"
                else
                    print_info "已取消"
                fi
            else
                print_warning "前端容器不存在: ${FRONTEND_CONTAINER_NAME}"
            fi
            ;;
        all)
            print_warning "这将删除所有容器"
            read -p "确认删除？(yes/no): " confirm
            if [ "$confirm" == "yes" ]; then
                docker rm -f "${FRONTEND_CONTAINER_NAME}" "${BACKEND_CONTAINER_NAME}" 2>/dev/null || true
                print_success "所有容器已删除"
            else
                print_info "已取消"
            fi
            ;;
        *)
            print_error "未知服务类型: $service_type"
            exit 1
            ;;
    esac
}

# ------------------------------------------------------------------------
# Service Management: Restart Containers
# ------------------------------------------------------------------------
restart_container() {
    print_header "重启 Docker 容器"
    
    local service_type="${1:-all}"
    local config_file="${2:-$DEFAULT_CONFIG_FILE}"
    
    stop_container "$service_type"
    sleep 2
    
    start_containers "$service_type" "$config_file"
}

# ------------------------------------------------------------------------
# Monitoring: Logs
# ------------------------------------------------------------------------
show_logs() {
    local service_type="${1:-all}"
    
    case "$service_type" in
        backend)
            if docker ps --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
                print_info "显示后端容器日志 (按 Ctrl+C 退出):"
                docker logs -f "${BACKEND_CONTAINER_NAME}"
            else
                print_warning "后端容器未运行: ${BACKEND_CONTAINER_NAME}"
                print_info "显示最后 100 行日志:"
                docker logs --tail 100 "${BACKEND_CONTAINER_NAME}" 2>/dev/null || print_error "无法获取日志"
            fi
            ;;
        frontend)
            if docker ps --format '{{.Names}}' | grep -q "^${FRONTEND_CONTAINER_NAME}$"; then
                print_info "显示前端容器日志 (按 Ctrl+C 退出):"
                docker logs -f "${FRONTEND_CONTAINER_NAME}"
            else
                print_warning "前端容器未运行: ${FRONTEND_CONTAINER_NAME}"
                print_info "显示最后 100 行日志:"
                docker logs --tail 100 "${FRONTEND_CONTAINER_NAME}" 2>/dev/null || print_error "无法获取日志"
            fi
            ;;
        all)
            if docker ps --format '{{.Names}}' | grep -qE "^(${BACKEND_CONTAINER_NAME}|${FRONTEND_CONTAINER_NAME})$"; then
                print_info "显示所有容器日志 (按 Ctrl+C 退出):"
                docker logs -f "${BACKEND_CONTAINER_NAME}" "${FRONTEND_CONTAINER_NAME}" 2>/dev/null || \
                docker logs -f "${BACKEND_CONTAINER_NAME}" 2>/dev/null || \
                docker logs -f "${FRONTEND_CONTAINER_NAME}" 2>/dev/null || \
                print_error "无法获取日志"
            else
                print_warning "没有运行中的容器"
            fi
            ;;
        *)
            print_error "未知服务类型: $service_type"
            exit 1
            ;;
    esac
}

# ------------------------------------------------------------------------
# Monitoring: Status
# ------------------------------------------------------------------------
show_status() {
    print_header "容器状态"
    
    print_info "容器状态:"
    docker ps -a --filter "name=${BACKEND_CONTAINER_NAME}" --filter "name=${FRONTEND_CONTAINER_NAME}" \
        --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | \
        grep -E "NAMES|${BACKEND_CONTAINER_NAME}|${FRONTEND_CONTAINER_NAME}"
    
    echo ""
    print_info "镜像信息:"
    docker images --filter "reference=${BACKEND_IMAGE_NAME}:${IMAGE_TAG}" \
        --filter "reference=${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}" \
        --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}"
    
    echo ""
    print_info "网络信息:"
    if docker network ls --format '{{.Name}}' | grep -q "^${NETWORK_NAME}$"; then
        print_success "网络已创建: ${NETWORK_NAME}"
    else
        print_warning "网络不存在: ${NETWORK_NAME}"
    fi
    
    echo ""
    print_info "健康检查:"
    
    # 后端健康检查
    if docker ps --format '{{.Names}}' | grep -q "^${BACKEND_CONTAINER_NAME}$"; then
        print_info "后端 (http://localhost:${BACKEND_PORT}/health):"
        if curl -s -f "http://localhost:${BACKEND_PORT}/health" > /dev/null 2>&1; then
            print_success "  后端 API 响应正常"
            curl -s "http://localhost:${BACKEND_PORT}/health" | jq '.' 2>/dev/null || curl -s "http://localhost:${BACKEND_PORT}/health"
        else
            print_error "  后端 API 无响应"
        fi
    else
        print_warning "  后端容器未运行"
    fi
    
    echo ""
    # 前端健康检查
    if docker ps --format '{{.Names}}' | grep -q "^${FRONTEND_CONTAINER_NAME}$"; then
        print_info "前端 (http://localhost:${FRONTEND_PORT}/health):"
        if curl -s -f "http://localhost:${FRONTEND_PORT}/health" > /dev/null 2>&1; then
            print_success "  前端服务响应正常"
        else
            print_error "  前端服务无响应"
        fi
    else
        print_warning "  前端容器未运行"
    fi
}

# ------------------------------------------------------------------------
# Maintenance: Clean Images
# ------------------------------------------------------------------------
clean_images() {
    print_header "清理 Docker 镜像"
    
    local clean_type="${1:-all}"
    
    case "$clean_type" in
        backend)
            print_warning "这将删除后端镜像: ${BACKEND_IMAGE_NAME}:${IMAGE_TAG}"
            read -p "确认删除？(yes/no): " confirm
            if [ "$confirm" == "yes" ]; then
                docker rmi "${BACKEND_IMAGE_NAME}:${IMAGE_TAG}" 2>/dev/null || print_warning "镜像不存在或无法删除"
                print_success "后端镜像清理完成"
            else
                print_info "已取消"
            fi
            ;;
        frontend)
            print_warning "这将删除前端镜像: ${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}"
            read -p "确认删除？(yes/no): " confirm
            if [ "$confirm" == "yes" ]; then
                docker rmi "${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}" 2>/dev/null || print_warning "镜像不存在或无法删除"
                print_success "前端镜像清理完成"
            else
                print_info "已取消"
            fi
            ;;
        all)
            print_warning "这将删除所有镜像"
            read -p "确认删除？(yes/no): " confirm
            if [ "$confirm" == "yes" ]; then
                docker rmi "${BACKEND_IMAGE_NAME}:${IMAGE_TAG}" "${FRONTEND_IMAGE_NAME}:${IMAGE_TAG}" 2>/dev/null || true
                print_success "所有镜像清理完成"
            else
                print_info "已取消"
            fi
            ;;
        *)
            print_error "未知清理类型: $clean_type"
            exit 1
            ;;
    esac
}

# ------------------------------------------------------------------------
# Help: Usage Information
# ------------------------------------------------------------------------
show_help() {
    echo "NOFX Docker 打包和启动脚本（前后端完整版）"
    echo ""
    echo "用法: ./docker-start.sh [command] [service] [options]"
    echo ""
    echo "命令:"
    echo "  build [backend|frontend|all]  构建 Docker 镜像（默认: all）"
    echo "  start [backend|frontend|all]  启动容器（默认: all）"
    echo "  stop [backend|frontend|all]   停止容器（默认: all）"
    echo "  restart [backend|frontend|all] 重启容器（默认: all）"
    echo "  remove [backend|frontend|all] 删除容器（默认: all）"
    echo "  logs [backend|frontend|all]   查看容器日志（默认: all）"
    echo "  status                        查看容器和镜像状态"
    echo "  clean [backend|frontend|all]  清理镜像（默认: all）"
    echo "  help                          显示此帮助信息"
    echo ""
    echo "选项:"
    echo "  --config PATH                 指定配置文件路径（相对于当前目录或绝对路径）"
    echo "                                在容器内挂载为 /app/config.json"
    echo ""
    echo "环境变量:"
    echo "  NOFX_BACKEND_IMAGE_NAME      后端镜像名称（默认: nofx-backend）"
    echo "  NOFX_FRONTEND_IMAGE_NAME     前端镜像名称（默认: nofx-frontend）"
    echo "  NOFX_IMAGE_TAG               镜像标签（默认: latest）"
    echo "  NOFX_BACKEND_CONTAINER_NAME   后端容器名称（默认: nofx-trading）"
    echo "  NOFX_FRONTEND_CONTAINER_NAME  前端容器名称（默认: nofx-frontend）"
    echo "  NOFX_NETWORK_NAME            Docker 网络名称（默认: nofx-network）"
    echo "  NOFX_CONFIG_FILE             默认配置文件路径（默认: ./config.json）"
    echo "  NOFX_BACKEND_PORT            后端 API 端口（默认: 8080）"
    echo "  NOFX_FRONTEND_PORT           前端端口（默认: 3000）"
    echo "  NOFX_DECISION_LOGS           决策日志目录（默认: ./decision_logs）"
    echo ""
    echo "示例:"
    echo "  # 构建所有镜像"
    echo "  ./docker-start.sh build"
    echo ""
    echo "  # 只构建后端镜像"
    echo "  ./docker-start.sh build backend"
    echo ""
    echo "  # 启动所有容器（前后端）"
    echo "  ./docker-start.sh start"
    echo ""
    echo "  # 只启动后端容器"
    echo "  ./docker-start.sh start backend"
    echo ""
    echo "  # 使用自定义配置文件启动"
    echo "  ./docker-start.sh start --config /path/to/my-config.json"
    echo ""
    echo "  # 查看后端日志"
    echo "  ./docker-start.sh logs backend"
    echo ""
    echo "  # 查看所有日志"
    echo "  ./docker-start.sh logs"
    echo ""
    echo "  # 查看状态"
    echo "  ./docker-start.sh status"
    echo ""
    echo "  # 构建并启动所有服务"
    echo "  ./docker-start.sh build && ./docker-start.sh start"
    echo ""
    echo "  # 使用环境变量自定义"
    echo "  NOFX_CONFIG_FILE=./prod-config.json NOFX_BACKEND_PORT=9090 ./docker-start.sh start"
}

# ------------------------------------------------------------------------
# Main: Command Dispatcher
# ------------------------------------------------------------------------
main() {
    check_docker
    
    local command="${1:-help}"
    local service_type="all"
    local config_file="$DEFAULT_CONFIG_FILE"
    local container_config_path="$CONTAINER_CONFIG_PATH"
    
    # 解析参数
    shift || true
    
    # 第一个参数可能是服务类型或选项
    if [[ $# -gt 0 ]] && [[ "$1" != --* ]]; then
        service_type="$1"
        shift || true
    fi
    
    # 解析选项
    while [[ $# -gt 0 ]]; do
        case $1 in
            --config)
                config_file="$2"
                shift 2
                ;;
            --container-config)
                container_config_path="$2"
                shift 2
                ;;
            *)
                print_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    case "$command" in
        build)
            build_images "$service_type"
            ;;
        start)
            start_containers "$service_type" "$config_file" "$container_config_path"
            ;;
        stop)
            stop_container "$service_type"
            ;;
        restart)
            restart_container "$service_type" "$config_file"
            ;;
        remove)
            remove_container "$service_type"
            ;;
        logs)
            show_logs "$service_type"
            ;;
        status)
            show_status
            ;;
        clean)
            clean_images "$service_type"
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_error "未知命令: $command"
            show_help
            exit 1
            ;;
    esac
}

# Execute Main
main "$@"

