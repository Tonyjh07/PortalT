package memory

import (
	"sync"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// UserRepository 基于内存的用户仓储实现。
type UserRepository struct {
	mu    sync.RWMutex
	users map[string]*domain.User
}

// NewUserRepository 创建内存用户仓储。
func NewUserRepository() *UserRepository {
	return &UserRepository{
		users: make(map[string]*domain.User),
	}
}

// Save 保存用户，ID 已存在时覆盖。
func (r *UserRepository) Save(user *domain.User) error {
	if user == nil || user.ID == "" {
		return ports.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[user.ID] = user
	return nil
}

// FindByID 按ID查找用户。
func (r *UserRepository) FindByID(id string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return user, nil
}

// FindByUsername 按用户名查找用户。
func (r *UserRepository) FindByUsername(username string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, user := range r.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, ports.ErrNotFound
}

// FindAll 返回全部用户，无数据时返回空切片。
func (r *UserRepository) FindAll() ([]*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	users := make([]*domain.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, user)
	}
	return users, nil
}

// Delete 删除用户。
func (r *UserRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[id]; !ok {
		return ports.ErrNotFound
	}
	delete(r.users, id)
	return nil
}
