package user

import (
	"fmt"
	"mime/multipart"
	"time"

	"github.com/chrisabs/storage/internal/models"
	"github.com/chrisabs/storage/internal/storage"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      *Repository
	jwtSecret string
}

func NewService(repo *Repository, jwtSecret string) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: jwtSecret,
	}
}

func (s *Service) generateJWT(userID int) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["userId"] = userID
	claims["exp"] = time.Now().Add(time.Hour * 24).Unix()

	return token.SignedString([]byte(s.jwtSecret))
}

func (s *Service) CreateUser(req *CreateUserRequest) (*models.User, error) {
	existingUser, err := s.repo.GetByEmail(req.Email)
	if err == nil && existingUser != nil {
		return nil, fmt.Errorf("email already exists")
	}

	user := &models.User{
		Email:     req.Email,
		Password:  req.Password,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		ImageURL:  req.ImageURL,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.repo.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %v", err)
	}

	return s.repo.GetByID(user.ID)
}

func (s *Service) Login(req *LoginRequest) (*AuthResponse, error) {
	user, err := s.repo.GetByEmail(req.Email)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	token, err := s.generateJWT(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %v", err)
	}

	return &AuthResponse{
		Token: token,
		User:  *user,
	}, nil
}

func (s *Service) GetUserByID(id int) (*models.User, error) {
	return s.repo.GetByID(id)
}

func (s *Service) GetAllUsers() ([]*models.User, error) {
	return s.repo.GetAll()
}

func (s *Service) UpdateUser(id int, firstName, lastName string, imageFile *multipart.FileHeader) (*models.User, error) {
    user, err := s.repo.GetByID(id)
    if err != nil {
        return nil, fmt.Errorf("user not found: %v", err)
    }

    user.FirstName = firstName
    user.LastName = lastName
    user.UpdatedAt = time.Now().UTC()

    if imageFile != nil {
        s3Handler, err := storage.NewS3Handler()
        if err != nil {
            return nil, fmt.Errorf("failed to initialize storage: %v", err)
        }

        imageURL, err := s3Handler.UploadFile(imageFile, fmt.Sprintf("users/%d", id))
        if err != nil {
            return nil, fmt.Errorf("failed to upload image: %v", err)
        }
        user.ImageURL = imageURL
    }

    if err := s.repo.Update(user); err != nil {
        return nil, fmt.Errorf("failed to update user: %v", err)
    }

    return s.repo.GetByID(id)
}

func (s *Service) DeleteUser(id int) error {
	return s.repo.Delete(id)
}
