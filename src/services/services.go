// services/services.go
package services

import (
	"context"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/go-redis/redis"
	"github.com/nabazesmail/gopher/src/initializers"
	"github.com/nabazesmail/gopher/src/middleware"
	"github.com/nabazesmail/gopher/src/models"
	"github.com/nabazesmail/gopher/src/repository"
	"github.com/nabazesmail/gopher/src/utils"
	"golang.org/x/crypto/bcrypt"
)

const (
	userCachePrefix = "user:"
	cacheExpiration = 10 * time.Minute // Cache expiration time
)

// Registering user
func CreateUser(body *models.User) (*models.User, error) {
	// Validate the input
	if body.FullName == "" || body.Username == "" || body.Password == "" {
		return nil, errors.New("all fields must be provided")
	}

	// Validate username using regex (allow only characters)
	usernameRegex := regexp.MustCompile("^[a-zA-Z]+$")
	if !usernameRegex.MatchString(body.Username) {
		middleware.Logger.Printf("username must contain only characters")
		return nil, errors.New("username must contain only characters")
	}

	if len(body.Password) < 8 || len(body.Password) > 15 {
		middleware.Logger.Printf("password must be between 8 and 15 characters")
		return nil, errors.New("password must be between 8 and 15 characters")
	}

	// Validate status and role (if provided)
	if body.Status != models.Active && body.Status != models.Inactive {
		middleware.Logger.Printf("invalid status value")
		return nil, errors.New("invalid status value")
	}

	if body.Role != models.Admin && body.Role != models.Operator {
		middleware.Logger.Printf("invalid Role value")
		return nil, errors.New("invalid role value")
	}

	// Hash the password using bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		middleware.Logger.Printf("Error hashing password: %s", err)
		return nil, err
	}

	// Create a new User instance with the hashed password
	user := &models.User{
		FullName: body.FullName,
		Username: body.Username,
		Password: string(hashedPassword),
		Status:   body.Status,
		Role:     body.Role,
	}

	// Save the user in the database
	err = repository.CreateUser(user)
	if err != nil {
		middleware.Logger.Printf("Error saving user in the database: %s", err)
		return nil, err
	}

	return user, nil
}

// getting all users
func GetAllUsers() ([]*models.User, error) {
	users, err := repository.GetAllUsers()
	if err != nil {
		middleware.Logger.Printf("Error retrieving users from the database: %s", err)
		return nil, err
	}

	return users, nil
}

// getting user by Id
func GetUserByID(userID string) (*models.User, error) {
	if userID == "" {
		return nil, errors.New("user ID must be provided")
	}

	// Check if the user is cached in Redis
	ctx := context.Background()
	cacheKey := userCachePrefix + userID
	cachedUser, err := initializers.RedisClient.Get(ctx, cacheKey).Result()
	if err == nil {
		// User found in cache, deserialize and return
		user, err := models.DeserializeUser(cachedUser)
		if err != nil {
			log.Printf("Error deserializing user data from cache: %s", err)
			// Proceed to fetch from the database
		} else {
			log.Printf("User with ID %s fetched from cache.", userID)
			return user, nil
		}
	} else if err != redis.Nil {
		log.Printf("Error fetching user from cache: %s", err)
		// Proceed to fetch from the database
	}

	// User not found in cache, fetch from the database
	user, err := repository.GetUserByID(userID)
	if err != nil {
		log.Printf("Error fetching user by ID: %s", err)
		return nil, err
	}

	if user == nil {
		return nil, nil // User not found
	}

	// Cache the user data in Redis
	serializedUser, err := user.Serialize()
	if err != nil {
		log.Printf("Error serializing user data for cache: %s", err)
	} else {
		_, err = initializers.RedisClient.Set(ctx, cacheKey, serializedUser, cacheExpiration).Result()
		if err != nil {
			log.Printf("Error caching user data: %s", err)
		} else {
			log.Printf("User with ID %s cached successfully.", userID)
		}
	}

	return user, nil
}

// updating user
func UpdateUserByID(userID string, body *models.User) (*models.User, error) {
	if userID == "" {
		return nil, errors.New("user ID must be provided")
	}

	user, err := repository.GetUserByID(userID)
	if err != nil {
		middleware.Logger.Printf("Error fetching user by ID: %s", err)
		return nil, err
	}

	if user == nil {
		return nil, nil // User not found
	}

	// Update user fields if they are provided in the request body
	if body.FullName != "" {
		user.FullName = body.FullName
	}

	if body.Username != "" {
		user.Username = body.Username
	}

	if body.Password != "" {
		// Hash the password using bcrypt
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing password: %s", err)
			return nil, err
		}
		user.Password = string(hashedPassword)
	}

	if body.Status != "" {
		user.Status = body.Status
	}

	if body.Role != "" {
		user.Role = body.Role
	}

	// Save the updated user in the database
	err = repository.UpdateUser(user)
	if err != nil {
		log.Printf("Error updating user: %s", err)
		return nil, err
	}

	return user, nil
}

// deleting user
func DeleteUserByID(userID string) error {
	if userID == "" {
		return errors.New("user ID must be provided")
	}

	user, err := repository.GetUserByID(userID)
	if err != nil {
		log.Printf("Error fetching user by ID: %s", err)
		return err
	}

	if user == nil {
		return nil // User not found
	}

	// Delete the user from the database
	err = repository.DeleteUser(user)
	if err != nil {
		log.Printf("Error deleting user: %s", err)
		return err
	}

	return nil
}

// authentication user
func AuthenticateUser(body *models.User) (string, error) {
	// Find the user by username in the database
	user, err := repository.GetUserByUsername(body.Username)
	if err != nil {
		middleware.Logger.Printf("Error fetching user by username: %s", err)
		return "", err
	}

	if user == nil {
		return "", errors.New("user not found")
	}

	// Compare the provided password with the hashed password in the database
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		log.Printf("Password verification failed for user %s: %s", user.Username, err)
		return "", errors.New("incorrect password")
	}

	// Generate a JWT token using the utils package
	tokenString, err := utils.GenerateJWTToken(user, []byte(os.Getenv("JWT_SECRET_KEY")))
	if err != nil {
		log.Printf("Error generating JWT token: %s", err)
		return "", errors.New("failed to generate JWT token")
	}

	return tokenString, nil
}

// UpdateUserProfilePicture updates the user's profile picture.
func UpdateUserProfilePicture(userID string, fileHeader *multipart.FileHeader) (*models.User, error) {
	// Find the user by ID in the database
	user, err := repository.GetUserByID(userID)
	if err != nil {
		middleware.Logger.Printf("Error fetching user by ID: %s", err)
		return nil, err
	}

	if user == nil {
		return nil, nil // User not found
	}

	// Check if the uploaded file is an image
	if !initializers.IsImageFile(fileHeader) {
		return nil, errors.New("invalid file format, only images are allowed")
	}

	// Create the file path for storing the uploaded image with the original filename
	filePath := filepath.Join("src/public/uploads", fileHeader.Filename)

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		middleware.Logger.Printf("Error opening uploaded file: %s", err)
		return nil, err
	}
	defer file.Close()

	// Create the destination file
	dst, err := os.Create(filePath)
	if err != nil {
		middleware.Logger.Printf("Error creating destination file: %s", err)
		return nil, err
	}
	defer dst.Close()

	// Copy the file data to the destination file
	_, err = io.Copy(dst, file)
	if err != nil {
		middleware.Logger.Printf("Error copying file data: %s", err)
		return nil, err
	}

	// Update the user's profile picture URL in the database with the original filename
	user.ProfilePicture = fileHeader.Filename
	if err := repository.UpdateUser(user); err != nil {
		middleware.Logger.Printf("Error updating user's profile picture: %s", err)
		return nil, err
	}

	return user, nil
}

// GetProfilePictureByID retrieves the user's profile picture by ID.
func GetProfilePictureByID(userID string) ([]byte, error) {
	// Find the user by ID in the database
	user, err := repository.GetUserByID(userID)
	if err != nil {
		middleware.Logger.Printf("Error fetching user by ID: %s", err)
		return nil, err
	}

	if user == nil {
		return nil, nil // User not found
	}

	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		middleware.Logger.Printf("Error getting current working directory: %s", err)
		return nil, err
	}

	// Get the absolute file path for the user's profile picture
	absoluteFilePath := filepath.Join(wd, "src/public/uploads", user.ProfilePicture)

	// Open the file
	file, err := os.Open(absoluteFilePath)
	if err != nil {
		middleware.Logger.Printf("Error opening profile picture file: %s", err)
		return nil, err
	}
	defer file.Close()

	// Read the file data
	data, err := io.ReadAll(file)
	if err != nil {
		middleware.Logger.Printf("Error reading profile picture data: %s", err)
		return nil, err
	}

	return data, nil
}

// PreviewProfilePicture fetches the binary data of the user's profile picture.
func PreviewProfilePicture(userID string) ([]byte, error) {
	// Find the user by ID in the database
	user, err := repository.GetUserByID(userID)
	if err != nil {
		middleware.Logger.Printf("Error fetching user by ID: %s", err)
		return nil, err
	}

	if user == nil {
		return nil, nil // User not found
	}

	// Construct the file path for the user's profile picture
	filePath := filepath.Join("src/public/uploads", user.ProfilePicture)

	// Read the file data
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		middleware.Logger.Printf("Error reading profile picture file: %s", err)
		return nil, err
	}

	return fileData, nil
}
