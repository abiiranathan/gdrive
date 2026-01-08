// Package gdrive provides a comprehensive client library for Google Drive API v3.
//
// This package offers a high-level interface for common Google Drive operations including:
//   - File uploads and downloads with streaming support
//   - Folder creation and file listing with full path resolution
//   - Partial downloads for resumable transfers
//   - Google Workspace document exports to various formats
//   - Trash operations (move to trash, restore, permanent delete)
//   - File revision management
//
// # Authentication
//
// The package supports two authentication methods:
//
// 1. OAuth2 for user authentication:
//
//	credentials, _ := os.ReadFile("credentials.json")
//	config, _ := gdrive.GetConfigFromJSON(credentials)
//	token := &oauth2.Token{AccessToken: "..."}
//	client, _ := gdrive.NewDriveClientWithToken(ctx, config, token)
//
// 2. Service Account for server-to-server:
//
//	credentials, _ := os.ReadFile("service-account.json")
//	client, _ := gdrive.NewDriveClientForServiceAccount(ctx, credentials)
//
// Basic Usage
//
//	// List files
//	files, err := client.ListFiles(ctx)
//	for _, file := range files {
//	    fmt.Printf("%s - %s\n", file.Name, file.FolderPath)
//	}
//
//	// Upload file
//	fileID, err := client.UploadFile(ctx, "/path/to/file.pdf", "file.pdf", "")
//
//	// Download file
//	bytesWritten, err := client.DownloadFile(ctx, fileID, "/path/to/output.pdf")
//
//	// Export Google Doc to PDF
//	err = client.ExportWorkspaceDocumentToFile(ctx, docID, "output.pdf", gdrive.ExportFormatPDF)
//
// # Thread Safety
//
// DriveClient is safe for concurrent use by multiple goroutines.
package gdrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// MaxPageSize is the maximum number of files to retrieve per API request.
// Google Drive API allows up to 1000, but 100 provides a good balance
// between API calls and memory usage.
const MaxPageSize = 100

// DriveClient wraps the Google Drive API client.
// It provides high-level methods for common Drive operations.
// Safe for concurrent use by multiple goroutines.
type DriveClient struct {
	service *drive.Service
}

// FileInfo represents metadata about a Google Drive file.
// This includes basic file information and the computed folder path.
type FileInfo struct {
	ID          string   // Unique file identifier in Google Drive
	Name        string   // Display name of the file
	MimeType    string   // MIME type (e.g., "application/pdf", "image/jpeg")
	Size        int64    // Size in bytes (0 for Google Workspace documents)
	WebViewLink string   // URL to view the file in a browser
	Parents     []string // List of parent folder IDs
	FolderPath  string   // Full folder path (e.g., "My Drive/Projects/2024")
}

// newDriveClient is the internal helper to initialize the Google Drive service.
// It creates a new drive.Service using the provided HTTP client.
//
// Parameters:
//   - ctx: Context for the API initialization
//   - client: Authenticated HTTP client with Drive API scope
//
// Returns:
//   - *DriveClient: Initialized client ready for use
//   - error: Any error encountered during service creation
func newDriveClient(ctx context.Context, client *http.Client) (*DriveClient, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive service: %w", err)
	}
	return &DriveClient{service: srv}, nil
}

// NewDriveClientForServiceAccount creates a DriveClient using Service Account credentials.
// This method is ideal for server-to-server interaction where no user interaction is needed.
// The service account must have access to the files/folders you want to access.
//
// To use this method:
//  1. Create a service account in Google Cloud Console
//  2. Download the JSON key file
//  3. Share Drive files/folders with the service account email
//
// Parameters:
//   - ctx: Context for the API initialization
//   - jsonCredentials: Contents of the service account JSON key file
//
// Returns:
//   - *DriveClient: Initialized client with read-only access
//   - error: Any error encountered during authentication or service creation
//
// Example:
//
//	credentials, _ := os.ReadFile("service-account.json")
//	client, err := gdrive.NewDriveClientForServiceAccount(ctx, credentials)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewDriveClientForServiceAccount(ctx context.Context, jsonCredentials []byte) (*DriveClient, error) {
	config, err := google.JWTConfigFromJSON(jsonCredentials, drive.DriveReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse service account credentials: %w", err)
	}
	client := config.Client(ctx)
	return newDriveClient(ctx, client)
}

// NewDriveClientWithToken creates a DriveClient using an existing OAuth2 token.
// This is the typical way a web application initializes the client after
// completing the OAuth2 authorization flow.
//
// Parameters:
//   - ctx: Context for the API initialization
//   - config: OAuth2 configuration (obtained from GetConfigFromJSON)
//   - tok: Valid OAuth2 token (obtained from OAuth2 flow)
//
// Returns:
//   - *DriveClient: Initialized client with user's Drive access
//   - error: Any error encountered during client creation
//
// Example:
//
//	config, _ := gdrive.GetConfigFromJSON(credentials)
//	token := &oauth2.Token{AccessToken: "...", RefreshToken: "..."}
//	client, err := gdrive.NewDriveClientWithToken(ctx, config, token)
func NewDriveClientWithToken(ctx context.Context, config *oauth2.Config, tok *oauth2.Token) (*DriveClient, error) {
	client := config.Client(ctx, tok)
	return newDriveClient(ctx, client)
}

// GetConfigFromJSON parses OAuth2 user credentials JSON into an oauth2.Config.
// This config is used to generate the authorization URL and exchange authorization
// codes for access tokens during the OAuth2 flow.
//
// To obtain credentials:
//  1. Go to Google Cloud Console
//  2. Create OAuth2 credentials (Desktop app or Web application)
//  3. Download the JSON file
//
// Parameters:
//   - jsonCredentials: Contents of the OAuth2 credentials JSON file
//
// Returns:
//   - *oauth2.Config: Configuration for OAuth2 flow
//   - error: Any error encountered during parsing
//
// Example:
//
//	credentials, _ := os.ReadFile("credentials.json")
//	config, err := gdrive.GetConfigFromJSON(credentials)
//	// Use config.AuthCodeURL() to start OAuth2 flow
func GetConfigFromJSON(jsonCredentials []byte) (*oauth2.Config, error) {
	return google.ConfigFromJSON(jsonCredentials, drive.DriveReadonlyScope)
}

// ListFiles retrieves all non-folder files from Google Drive with folder path information.
// This method fetches files across all folders and computes the full folder path for each file.
// Files are retrieved in pages of MaxPageSize (100) items.
//
// Note: This method skips:
//   - Folders (mimeType: "application/vnd.google-apps.folder")
//   - Zero-byte files (likely corrupted or placeholders)
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//
// Returns:
//   - []FileInfo: Slice of file metadata with folder paths
//   - error: Any error encountered during API calls
//
// Example:
//
//	files, err := client.ListFiles(ctx)
//	for _, file := range files {
//	    fmt.Printf("%s (%d bytes) - %s\n", file.Name, file.Size, file.FolderPath)
//	}
func (dc *DriveClient) ListFiles(ctx context.Context) ([]FileInfo, error) {
	files := make([]FileInfo, 0, MaxPageSize)
	pageToken := ""

	// Build a map of folder IDs to folder names for path resolution
	folderMap := make(map[string]string)

	// Fetch all folders
	foldersCall := dc.service.Files.List().
		Context(ctx).
		Q("mimeType='application/vnd.google-apps.folder'").
		Fields("files(id, name, parents)").
		PageSize(1000)

	foldersResp, err := foldersCall.Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve folders: %w", err)
	}

	for _, folder := range foldersResp.Files {
		folderMap[folder.Id] = folder.Name
	}

	// Helper function to build full folder path
	buildPath := func(parentIDs []string) string {
		if len(parentIDs) == 0 {
			return "My Drive"
		}

		var pathParts []string
		currentID := parentIDs[0]
		visited := make(map[string]bool)

		// Traverse up the folder hierarchy (max 10 levels to prevent infinite loops)
		for i := 0; i < 10 && currentID != "" && !visited[currentID]; i++ {
			visited[currentID] = true
			if folderName, exists := folderMap[currentID]; exists {
				pathParts = append([]string{folderName}, pathParts...)
				// Find parent of current folder
				for _, folder := range foldersResp.Files {
					if folder.Id == currentID && len(folder.Parents) > 0 {
						currentID = folder.Parents[0]
						break
					}
				}
			} else {
				break
			}
		}

		if len(pathParts) == 0 {
			return "My Drive"
		}
		return "My Drive/" + strings.Join(pathParts, "/")
	}

	// Fetch all files in pages
	for {
		call := dc.service.Files.List().
			Context(ctx).
			PageSize(MaxPageSize).
			Fields("nextPageToken, files(id, name, mimeType, size, webViewLink, parents)")

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		r, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve files: %w", err)
		}

		for _, item := range r.Files {
			// Skip folders and zero-byte files
			if item.Size == 0 || item.MimeType == "application/vnd.google-apps.folder" {
				continue
			}

			files = append(files, FileInfo{
				ID:          item.Id,
				Name:        item.Name,
				MimeType:    item.MimeType,
				Size:        item.Size,
				WebViewLink: item.WebViewLink,
				Parents:     item.Parents,
				FolderPath:  buildPath(item.Parents),
			})
		}

		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return files, nil
}

// ListFilesInFolder retrieves all non-folder files from a specific Google Drive folder.
// This method is more efficient than ListFiles when you only need files from one folder.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - parentFolderID: ID of the parent folder. Empty string lists root-level files in "My Drive"
//
// Returns:
//   - []FileInfo: Slice of file metadata with folder paths
//   - error: Any error encountered during API calls
//
// Example:
//
//	// List files in a specific folder
//	files, err := client.ListFilesInFolder(ctx, "1aBc2DeFg3HiJ4KlM5nOp")
//
//	// List files in root of My Drive
//	files, err := client.ListFilesInFolder(ctx, "")
func (dc *DriveClient) ListFilesInFolder(ctx context.Context, parentFolderID string) ([]FileInfo, error) {
	files := make([]FileInfo, 0, MaxPageSize)
	pageToken := ""

	// Build query to filter by parent folder
	query := "trashed=false"
	if parentFolderID != "" {
		query = fmt.Sprintf("'%s' in parents and trashed=false", parentFolderID)
	}

	// Build folder maps for path resolution
	folderMap := make(map[string]string)
	folderParentMap := make(map[string][]string)

	foldersCall := dc.service.Files.List().
		Context(ctx).
		Q("mimeType='application/vnd.google-apps.folder'").
		Fields("files(id, name, parents)").
		PageSize(1000)

	foldersResp, err := foldersCall.Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve folders: %w", err)
	}

	for _, folder := range foldersResp.Files {
		folderMap[folder.Id] = folder.Name
		folderParentMap[folder.Id] = folder.Parents
	}

	// Helper function to build folder path
	buildPath := func(parentIDs []string) string {
		if len(parentIDs) == 0 {
			return "My Drive"
		}

		var pathParts []string
		currentID := parentIDs[0]
		visited := make(map[string]bool)

		for i := 0; i < 10 && currentID != "" && !visited[currentID]; i++ {
			visited[currentID] = true
			if folderName, exists := folderMap[currentID]; exists {
				pathParts = append([]string{folderName}, pathParts...)
				if parents, hasParent := folderParentMap[currentID]; hasParent && len(parents) > 0 {
					currentID = parents[0]
				} else {
					break
				}
			} else {
				break
			}
		}

		if len(pathParts) == 0 {
			return "My Drive"
		}
		return "My Drive/" + strings.Join(pathParts, "/")
	}

	// Fetch files
	for {
		call := dc.service.Files.List().
			Context(ctx).
			Q(query).
			PageSize(MaxPageSize).
			Fields("nextPageToken, files(id, name, mimeType, size, webViewLink, parents)")

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		r, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve files: %w", err)
		}

		for _, item := range r.Files {
			if item.MimeType == "application/vnd.google-apps.folder" || item.Size == 0 {
				continue
			}

			files = append(files, FileInfo{
				ID:          item.Id,
				Name:        item.Name,
				MimeType:    item.MimeType,
				Size:        item.Size,
				WebViewLink: item.WebViewLink,
				Parents:     item.Parents,
				FolderPath:  buildPath(item.Parents),
			})
		}

		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return files, nil
}

// StreamFile downloads a file from Google Drive and streams its content to the provided io.Writer.
// This is highly efficient for large files and web responses (e.g., http.ResponseWriter).
// The entire file content is copied to the writer without loading it into memory.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: Unique Google Drive file identifier
//   - w: Destination writer (e.g., os.File, bytes.Buffer, http.ResponseWriter)
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during download or streaming
//
// Example:
//
//	// Stream to HTTP response
//	bytesWritten, err := client.StreamFile(ctx, fileID, w)
//
//	// Stream to buffer
//	var buf bytes.Buffer
//	bytesWritten, err := client.StreamFile(ctx, fileID, &buf)
func (dc *DriveClient) StreamFile(ctx context.Context, fileID string, w io.Writer) (int64, error) {
	if fileID == "" {
		return 0, errors.New("file ID cannot be empty")
	}

	resp, err := dc.service.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return 0, fmt.Errorf("unable to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		return written, fmt.Errorf("unable to stream file content: %w", err)
	}

	return written, nil
}

// DownloadFile downloads a file from Google Drive to a local file path.
// The parent directory is created automatically if it doesn't exist.
// This is a convenience wrapper around StreamFile for file-based downloads.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: Unique Google Drive file identifier
//   - outputPath: Local file system path where file will be saved
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during download or file creation
//
// Example:
//
//	bytesWritten, err := client.DownloadFile(ctx, "1aBc2DeF", "/downloads/document.pdf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Downloaded %d bytes\n", bytesWritten)
func (dc *DriveClient) DownloadFile(ctx context.Context, fileID, outputPath string) (int64, error) {
	if outputPath == "" {
		return 0, errors.New("output path cannot be empty")
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("unable to create output directory: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("unable to create output file: %w", err)
	}
	defer out.Close()

	written, err := dc.StreamFile(ctx, fileID, out)
	if err != nil {
		return written, fmt.Errorf("unable to download file: %w", err)
	}

	return written, nil
}

// UploadFile uploads a local file to Google Drive.
// The MIME type is automatically detected from the file content.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - filePath: Path to the local file to upload
//   - fileName: Display name in Google Drive. If empty, uses the basename of filePath
//   - parentFolderID: ID of the parent folder. Empty string uploads to "My Drive" root
//
// Returns:
//   - string: File ID of the uploaded file in Google Drive
//   - error: Any error encountered during upload
//
// Example:
//
//	// Upload to root of My Drive
//	fileID, err := client.UploadFile(ctx, "/docs/report.pdf", "Q4 Report.pdf", "")
//
//	// Upload to specific folder
//	fileID, err := client.UploadFile(ctx, "/docs/report.pdf", "Q4 Report.pdf", "folderID123")
func (dc *DriveClient) UploadFile(ctx context.Context, filePath, fileName, parentFolderID string) (string, error) {
	if filePath == "" {
		return "", errors.New("file path cannot be empty")
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("unable to stat file: %w", err)
	}

	// Detect MIME type
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("unable to read file for MIME detection: %w", err)
	}
	mimeType := http.DetectContentType(buffer[:n])

	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("unable to reset file pointer: %w", err)
	}

	fileMeta := &drive.File{
		Name:     fileName,
		MimeType: mimeType,
	}

	if parentFolderID != "" {
		fileMeta.Parents = []string{parentFolderID}
	}

	uploadedFile, err := dc.service.Files.Create(fileMeta).
		Context(ctx).
		Media(file).
		Fields("id, name, mimeType, size, parents, webViewLink").
		Do()
	if err != nil {
		return "", fmt.Errorf("unable to upload file: %w", err)
	}

	fmt.Printf("File uploaded successfully: %s (ID: %s, Size: %d bytes)\n",
		uploadedFile.Name, uploadedFile.Id, fileInfo.Size())

	return uploadedFile.Id, nil
}

// UploadFileFromReader uploads a file to Google Drive from an io.Reader.
// This is particularly useful for web applications to upload files directly
// from HTTP requests without saving to disk first.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - reader: Source reader containing file content
//   - fileName: Display name in Google Drive (required)
//   - mimeType: MIME type of the file. Use "application/octet-stream" if unknown
//   - parentFolderID: ID of the parent folder. Empty string uploads to "My Drive" root
//
// Returns:
//   - string: File ID of the uploaded file in Google Drive
//   - error: Any error encountered during upload
//
// Example:
//
//	// Upload from HTTP request
//	file, header, _ := r.FormFile("upload")
//	defer file.Close()
//	fileID, err := client.UploadFileFromReader(ctx, file, header.Filename,
//	    header.Header.Get("Content-Type"), "")
func (dc *DriveClient) UploadFileFromReader(ctx context.Context, reader io.Reader, fileName, mimeType, parentFolderID string) (string, error) {
	if reader == nil {
		return "", errors.New("reader cannot be nil")
	}
	if fileName == "" {
		return "", errors.New("file name cannot be empty")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	fileMeta := &drive.File{
		Name:     fileName,
		MimeType: mimeType,
	}

	if parentFolderID != "" {
		fileMeta.Parents = []string{parentFolderID}
	}

	uploadedFile, err := dc.service.Files.Create(fileMeta).
		Context(ctx).
		Media(reader).
		Fields("id, name, mimeType, size, parents, webViewLink").
		Do()
	if err != nil {
		return "", fmt.Errorf("unable to upload file: %w", err)
	}

	fmt.Printf("File uploaded successfully: %s (ID: %s)\n", uploadedFile.Name, uploadedFile.Id)
	return uploadedFile.Id, nil
}

// CreateFolder creates a new folder in Google Drive.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - folderName: Name of the folder to create (required)
//   - parentFolderID: ID of the parent folder. Empty string creates in "My Drive" root
//
// Returns:
//   - string: Folder ID of the created folder
//   - error: Any error encountered during creation
//
// Example:
//
//	// Create folder in root
//	folderID, err := client.CreateFolder(ctx, "Project Files", "")
//
//	// Create subfolder
//	subfolderID, err := client.CreateFolder(ctx, "2024", folderID)
func (dc *DriveClient) CreateFolder(ctx context.Context, folderName, parentFolderID string) (string, error) {
	if folderName == "" {
		return "", errors.New("folder name cannot be empty")
	}

	folderMeta := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
	}

	if parentFolderID != "" {
		folderMeta.Parents = []string{parentFolderID}
	}

	folder, err := dc.service.Files.Create(folderMeta).
		Context(ctx).
		Fields("id, name").
		Do()
	if err != nil {
		return "", fmt.Errorf("unable to create folder: %w", err)
	}

	fmt.Printf("Folder created successfully: %s (ID: %s)\n", folder.Name, folder.Id)
	return folder.Id, nil
}

// TrashFile moves a file or folder to the trash in Google Drive.
// Trashed items can be restored using RestoreFile or permanently deleted
// from the Google Drive web interface.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file or folder to trash
//
// Returns:
//   - error: Any error encountered during the operation
//
// Example:
//
//	err := client.TrashFile(ctx, "1aBc2DeF")
//	if err != nil {
//	    log.Printf("Failed to trash file: %v", err)
//	}
func (dc *DriveClient) TrashFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return errors.New("file ID cannot be empty")
	}

	_, err := dc.service.Files.Update(fileID, &drive.File{
		Trashed: true,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("unable to trash file: %w", err)
	}

	fmt.Printf("File moved to trash: %s\n", fileID)
	return nil
}

// RestoreFile restores a file or folder from the trash in Google Drive.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file or folder to restore
//
// Returns:
//   - error: Any error encountered during the operation
//
// Example:
//
//	err := client.RestoreFile(ctx, "1aBc2DeF")
func (dc *DriveClient) RestoreFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return errors.New("file ID cannot be empty")
	}

	_, err := dc.service.Files.Update(fileID, &drive.File{
		Trashed: false,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("unable to restore file: %w", err)
	}

	fmt.Printf("File restored from trash: %s\n", fileID)
	return nil
}

// DeleteFile permanently deletes a file or folder from Google Drive.
// WARNING: This action is irreversible. The file cannot be recovered.
// Consider using TrashFile instead for recoverable deletion.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file or folder to permanently delete
//
// Returns:
//   - error: Any error encountered during the operation
//
// Example:
//
//	err := client.DeleteFile(ctx, "1aBc2DeF")
//	if err != nil {
//	    log.Printf("Failed to delete file: %v", err)
//	}
func (dc *DriveClient) DeleteFile(ctx context.Context, fileID string) error {
	if fileID == "" {
		return errors.New("file ID cannot be empty")
	}

	err := dc.service.Files.Delete(fileID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("unable to delete file permanently: %w", err)
	}

	fmt.Printf("File permanently deleted: %s\n", fileID)
	return nil
}

// PartialDownloadOptions specifies options for downloading a specific byte range of a file.
// Useful for resumable downloads, streaming large files in chunks, or implementing
// range requests for media serving.
type PartialDownloadOptions struct {
	StartByte int64 // Starting byte position (inclusive, zero-based)
	EndByte   int64 // Ending byte position (inclusive)
}

// PartialDownloadFile downloads a specific byte range of a file from Google Drive.
// This is useful for resumable downloads, streaming large files in chunks,
// or implementing HTTP range requests.
//
// Note: Partial downloads are not supported for Google Workspace documents
// (Google Docs, Sheets, Slides, etc.). Use ExportWorkspaceDocument instead.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file to download
//   - w: Destination writer for the file content
//   - opts: Byte range options specifying start and end positions
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during download
//
// Example:
//
//	// Download first 1 MB
//	opts := gdrive.PartialDownloadOptions{StartByte: 0, EndByte: 1048575}
//	bytesWritten, err := client.PartialDownloadFile(ctx, fileID, &buf, opts)
//
//	// Resume download from byte 1048576
//	opts = gdrive.PartialDownloadOptions{StartByte: 1048576, EndByte: 2097151}
//	bytesWritten, err = client.PartialDownloadFile(ctx, fileID, &buf, opts)
func (dc *DriveClient) PartialDownloadFile(ctx context.Context, fileID string, w io.Writer, opts PartialDownloadOptions) (int64, error) {
	if fileID == "" {
		return 0, errors.New("file ID cannot be empty")
	}
	if opts.StartByte < 0 || opts.EndByte < 0 {
		return 0, errors.New("byte positions cannot be negative")
	}
	if opts.StartByte > opts.EndByte {
		return 0, errors.New("start byte must be less than or equal to end byte")
	}

	call := dc.service.Revisions.Get(fileID, fileID).Context(ctx)
	rangeHeader := fmt.Sprintf("bytes=%d-%d", opts.StartByte, opts.EndByte)
	call.Header().Set("Range", rangeHeader)

	resp, err := call.Download()
	if err != nil {
		return 0, fmt.Errorf("unable to download revision: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		return written, fmt.Errorf("unable to write revision content: %w", err)
	}

	return written, nil
}

// PartialStreamFile is a convenience wrapper around PartialDownloadFile.
// Downloads a specific byte range of a file to the provided writer.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file to download
//   - w: Destination writer for the file content
//   - startByte: Starting byte position (inclusive, zero-based)
//   - endByte: Ending byte position (inclusive)
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during download
//
// Example:
//
//	// Download bytes 0-1023 (first 1024 bytes)
//	bytesWritten, err := client.PartialStreamFile(ctx, fileID, &buf, 0, 1023)
func (dc *DriveClient) PartialStreamFile(ctx context.Context, fileID string, w io.Writer, startByte, endByte int64) (int64, error) {
	return dc.PartialDownloadFile(ctx, fileID, w, PartialDownloadOptions{
		StartByte: startByte,
		EndByte:   endByte,
	})
}

// ExportFormat represents supported export formats for Google Workspace documents.
// Different document types support different export formats.
type ExportFormat string

// Export format constants for Google Workspace documents.
const (
	ExportFormatPDF  ExportFormat = "application/pdf"                                                           // PDF (all types)
	ExportFormatDOCX ExportFormat = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"   // Word (Docs)
	ExportFormatXLSX ExportFormat = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"         // Excel (Sheets)
	ExportFormatPPTX ExportFormat = "application/vnd.openxmlformats-officedocument.presentationml.presentation" // PowerPoint (Slides)
	ExportFormatODT  ExportFormat = "application/vnd.oasis.opendocument.text"                                   // OpenDocument Text (Docs)
	ExportFormatODS  ExportFormat = "application/vnd.oasis.opendocument.spreadsheet"                            // OpenDocument Spreadsheet (Sheets)
	ExportFormatODP  ExportFormat = "application/vnd.oasis.opendocument.presentation"                           // OpenDocument Presentation (Slides)
	ExportFormatRTF  ExportFormat = "application/rtf"                                                           // Rich Text Format (Docs)
	ExportFormatTXT  ExportFormat = "text/plain"                                                                // Plain text (Docs)
	ExportFormatHTML ExportFormat = "text/html"                                                                 // HTML (Docs, Sheets)
	ExportFormatZIP  ExportFormat = "application/zip"                                                           // ZIP (Docs, Sheets, Slides)
	ExportFormatJPEG ExportFormat = "image/jpeg"                                                                // JPEG (Drawings, Slides)
	ExportFormatPNG  ExportFormat = "image/png"                                                                 // PNG (Drawings, Slides)
	ExportFormatSVG  ExportFormat = "image/svg+xml"                                                             // SVG (Drawings)
	ExportFormatCSV  ExportFormat = "text/csv"                                                                  // CSV (Sheets)
	ExportFormatEPUB ExportFormat = "application/epub+zip"                                                      // EPUB (Docs)
)

// ExportWorkspaceDocument exports a Google Workspace document to the specified format.
// Supported formats depend on the document type:
//   - Google Docs: PDF, DOCX, ODT, RTF, TXT, HTML, EPUB, ZIP
//   - Google Sheets: PDF, XLSX, ODS, CSV, HTML, ZIP
//   - Google Slides: PDF, PPTX, ODP, TXT, JPEG, PNG, SVG
//   - Google Drawings: PDF, JPEG, PNG, SVG
//
// Note: Exported content is limited to 10 MB by Google Drive API.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the Google Workspace document
//   - w: Destination writer for the exported content
//   - format: Desired export format (use ExportFormat constants)
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during export
//
// Example:
//
//	// Export Google Doc to PDF
//	var buf bytes.Buffer
//	bytesWritten, err := client.ExportWorkspaceDocument(ctx, docID, &buf, gdrive.ExportFormatPDF)
//
//	// Export Google Sheet to Excel
//	bytesWritten, err := client.ExportWorkspaceDocument(ctx, sheetID, &buf, gdrive.ExportFormatXLSX)
func (dc *DriveClient) ExportWorkspaceDocument(ctx context.Context, fileID string, w io.Writer, format ExportFormat) (int64, error) {
	if fileID == "" {
		return 0, errors.New("file ID cannot be empty")
	}
	if format == "" {
		return 0, errors.New("export format cannot be empty")
	}

	resp, err := dc.service.Files.Export(fileID, string(format)).Context(ctx).Download()
	if err != nil {
		return 0, fmt.Errorf("unable to export document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		return written, fmt.Errorf("unable to write exported content: %w", err)
	}
	return written, nil
}

// ExportWorkspaceDocumentToFile exports a Google Workspace document to a local file.
// This is a convenience method that wraps ExportWorkspaceDocument.
// The parent directory is created automatically if it doesn't exist.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the Google Workspace document
//   - outputPath: Local file system path where exported file will be saved
//   - format: Desired export format (use ExportFormat constants)
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during export or file creation
//
// Example:
//
//	// Export Google Doc to PDF file
//	bytesWritten, err := client.ExportWorkspaceDocumentToFile(ctx, docID,
//	    "/exports/document.pdf", gdrive.ExportFormatPDF)
func (dc *DriveClient) ExportWorkspaceDocumentToFile(ctx context.Context, fileID, outputPath string, format ExportFormat) (int64, error) {
	if outputPath == "" {
		return 0, errors.New("output path cannot be empty")
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("unable to create output directory: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("unable to create output file: %w", err)
	}
	defer out.Close()

	written, err := dc.ExportWorkspaceDocument(ctx, fileID, out, format)
	if err != nil {
		return written, fmt.Errorf("unable to export document: %w", err)
	}

	return written, nil
}

// GetExportLinks retrieves all available export links for a Google Workspace document.
// This returns a map of MIME types to direct download URLs that can be used
// to download the document in various formats.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the Google Workspace document
//
// Returns:
//   - map[string]string: Map of MIME type to download URL
//   - error: Error if file is not a Workspace document or API call fails
//
// Example:
//
//	links, err := client.GetExportLinks(ctx, docID)
//	for mimeType, url := range links {
//	    fmt.Printf("%s: %s\n", mimeType, url)
//	}
//	// Output might be:
//	// application/pdf: https://docs.google.com/...
//	// application/vnd.openxmlformats-officedocument.wordprocessingml.document: https://docs.google.com/...
func (dc *DriveClient) GetExportLinks(ctx context.Context, fileID string) (map[string]string, error) {
	if fileID == "" {
		return nil, errors.New("file ID cannot be empty")
	}

	file, err := dc.service.Files.Get(fileID).
		Context(ctx).
		Fields("exportLinks, mimeType").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get file metadata: %w", err)
	}

	if len(file.ExportLinks) == 0 {
		return nil, fmt.Errorf("file is not a Google Workspace document (MIME type: %s)", file.MimeType)
	}
	return file.ExportLinks, nil
}

// DownloadRevision downloads a specific revision of a file.
// The revision must be marked as "Keep Forever" in Google Drive to be downloadable.
// This is useful for version control and accessing historical versions of files.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file
//   - revisionID: ID of the specific revision to download
//   - w: Destination writer for the revision content
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during download
//
// Example:
//
//	var buf bytes.Buffer
//	bytesWritten, err := client.DownloadRevision(ctx, fileID, revisionID, &buf)
func (dc *DriveClient) DownloadRevision(ctx context.Context, fileID, revisionID string, w io.Writer) (int64, error) {
	if fileID == "" {
		return 0, errors.New("file ID cannot be empty")
	}
	if revisionID == "" {
		return 0, errors.New("revision ID cannot be empty")
	}

	resp, err := dc.service.Revisions.Get(fileID, revisionID).
		Context(ctx).
		Download()
	if err != nil {
		return 0, fmt.Errorf("unable to download revision: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		return written, fmt.Errorf("unable to write revision content: %w", err)
	}

	return written, nil
}

// PartialDownloadRevision downloads a specific byte range of a file revision.
// The revision must be marked as "Keep Forever" in Google Drive to be downloadable.
// Useful for resumable downloads of historical file versions.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file
//   - revisionID: ID of the specific revision to download
//   - w: Destination writer for the revision content
//   - opts: Byte range options specifying start and end positions
//
// Returns:
//   - int64: Number of bytes written
//   - error: Any error encountered during download
//
// Example:
//
//	opts := gdrive.PartialDownloadOptions{StartByte: 0, EndByte: 1023}
//	bytesWritten, err := client.PartialDownloadRevision(ctx, fileID, revisionID, &buf, opts)
func (dc *DriveClient) PartialDownloadRevision(ctx context.Context, fileID, revisionID string, w io.Writer, opts PartialDownloadOptions) (int64, error) {
	if fileID == "" {
		return 0, errors.New("file ID cannot be empty")
	}
	if revisionID == "" {
		return 0, errors.New("revision ID cannot be empty")
	}
	if opts.StartByte < 0 || opts.EndByte < 0 {
		return 0, errors.New("byte positions cannot be negative")
	}
	if opts.StartByte > opts.EndByte {
		return 0, errors.New("start byte must be less than or equal to end byte")
	}

	call := dc.service.Revisions.Get(fileID, revisionID).Context(ctx)
	rangeHeader := fmt.Sprintf("bytes=%d-%d", opts.StartByte, opts.EndByte)
	call.Header().Set("Range", rangeHeader)

	resp, err := call.Download()
	if err != nil {
		return 0, fmt.Errorf("unable to download revision: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		return written, fmt.Errorf("unable to write revision content: %w", err)
	}

	return written, nil
}

// IsWorkspaceDocument checks if a file is a Google Workspace document.
// Returns true for Google Docs, Sheets, Slides, Forms, Drawings, etc.
// Returns false for regular files and folders.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - fileID: ID of the file to check
//
// Returns:
//   - bool: true if file is a Google Workspace document, false otherwise
//   - error: Any error encountered during the API call
//
// Example:
//
//	isWorkspace, err := client.IsWorkspaceDocument(ctx, fileID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if isWorkspace {
//	    // Use ExportWorkspaceDocument instead of DownloadFile
//	    client.ExportWorkspaceDocument(ctx, fileID, &buf, gdrive.ExportFormatPDF)
//	} else {
//	    // Regular file download
//	    client.DownloadFile(ctx, fileID, "output.bin")
//	}
func (dc *DriveClient) IsWorkspaceDocument(ctx context.Context, fileID string) (bool, error) {
	if fileID == "" {
		return false, errors.New("file ID cannot be empty")
	}

	file, err := dc.service.Files.Get(fileID).
		Context(ctx).
		Fields("mimeType").
		Do()
	if err != nil {
		return false, fmt.Errorf("unable to get file metadata: %w", err)
	}

	// Google Workspace MIME types start with "application/vnd.google-apps."
	isWorkspace := len(file.MimeType) > 28 && file.MimeType[:28] == "application/vnd.google-apps."

	// Exclude folders
	if file.MimeType == "application/vnd.google-apps.folder" {
		return false, nil
	}

	return isWorkspace, nil
}
