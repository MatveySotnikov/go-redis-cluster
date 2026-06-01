package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"example.com/pz9-redis-cache/internal/cache"
	"example.com/pz9-redis-cache/internal/config"
	"example.com/pz9-redis-cache/internal/task"
	"github.com/redis/go-redis/v9"
)

type TaskService struct {
	repo  *task.Repo
	redis *redis.Client
	cfg   config.Config
}

func NewTaskService(repo *task.Repo, redisClient *redis.Client, cfg config.Config) *TaskService {
	return &TaskService{
		repo:  repo,
		redis: redisClient,
		cfg:   cfg,
	}
}

func (s *TaskService) GetTaskByID(ctx context.Context, id int64) (task.Task, error) {
	key := cache.TaskByIDKey(id)

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, key).Result()
		if err == nil {
			var t task.Task
			if err := json.Unmarshal([]byte(cached), &t); err == nil {
				log.Println("[CACHE] hit:", key)
				return t, nil
			}
			log.Println("[CACHE] decode error:", err)
		} else if !errors.Is(err, redis.Nil) {
			log.Println("[CACHE] read error:", err)
		} else {
			log.Println("[CACHE] miss:", key)
		}
	}

	t, err := s.repo.GetByID(id)
	if err != nil {
		return task.Task{}, err
	}

	if s.redis != nil {
		bytes, err := json.Marshal(t)
		if err != nil {
			log.Println("[CACHE] encode error:", err)
			return t, nil
		}

		ttl := cache.TTLWithJitter(s.cfg.CacheTTL, s.cfg.CacheTTLJitter)
		if err := s.redis.Set(ctx, key, bytes, ttl).Err(); err != nil {
			log.Println("[CACHE] write error:", err)
		} else {
			log.Println("[CACHE] set:", key)
		}
	}

	return t, nil
}

func (s *TaskService) GetAllTasks(ctx context.Context) ([]task.Task, error) {
	key := cache.TasksListKey()

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, key).Result()
		if err == nil {
			var tasks []task.Task
			if err := json.Unmarshal([]byte(cached), &tasks); err == nil {
				log.Println("[CACHE] hit (list):", key)
				return tasks, nil
			}
			log.Println("[CACHE] decode error (list):", err)
		} else if !errors.Is(err, redis.Nil) {
			log.Println("[CACHE] read error (list):", err)
		} else {
			log.Println("[CACHE] miss (list):", key)
		}
	}

	tasks, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		bytes, err := json.Marshal(tasks)
		if err != nil {
			log.Println("[CACHE] encode error (list):", err)
			return tasks, nil
		}

		// Для списка TTL можно сделать короче, используем базовый без джиттера или с уменьшенным джиттером
		ttl := cache.TTLWithJitter(s.cfg.CacheTTL/2, s.cfg.CacheTTLJitter/2)
		if err := s.redis.Set(ctx, key, bytes, ttl).Err(); err != nil {
			log.Println("[CACHE] write error (list):", err)
		} else {
			log.Println("[CACHE] set (list):", key)
		}
	}

	return tasks, nil
}

func (s *TaskService) UpdateTask(ctx context.Context, t task.Task) error {
	if err := s.repo.Update(t); err != nil {
		return err
	}

	if s.redis != nil {
		// Инвалидация кэша конкретной задачи и списка
		taskKey := cache.TaskByIDKey(t.ID)
		listKey := cache.TasksListKey()

		if err := s.redis.Del(ctx, taskKey, listKey).Err(); err != nil {
			log.Println("[CACHE] delete error:", err)
		} else {
			log.Println("[CACHE] invalidated:", taskKey, "and", listKey)
		}
	}
	return nil
}

func (s *TaskService) DeleteTask(ctx context.Context, id int64) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}

	if s.redis != nil {
		taskKey := cache.TaskByIDKey(id)
		listKey := cache.TasksListKey()

		if err := s.redis.Del(ctx, taskKey, listKey).Err(); err != nil {
			log.Println("[CACHE] delete error:", err)
		} else {
			log.Println("[CACHE] invalidated:", taskKey, "and", listKey)
		}
	}
	return nil
}
