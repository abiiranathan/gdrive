# Examples

Comprehensive examples for using the gdrive package.

## Table of Contents

- [Authentication](#authentication)
- [Listing Files](#listing-files)
- [Uploading Files](#uploading-files)
- [Downloading Files](#downloading-files)
- [Working with Folders](#working-with-folders)
- [Google Workspace Documents](#google-workspace-documents)
- [Partial Downloads](#partial-downloads)
- [Web Application Integration](#web-application-integration)
- [Error Handling](#error-handling)

## Authentication

### OAuth2 Flow (Web Application)

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"

    "github.com/abiiranathan/gdrive"
    "golang.org/x/oauth2"
)

func main() {
    ctx := context.Background()

    // Read OAuth2 credentials
    credentials, err := os.ReadFile("credentials.json")
    if err != nil {
        log.Fatalf("Unable to read credentials: %v", err)
    }

    // Parse credentials
    config, err := gdrive.GetConfigFromJSON(credentials)
    if err != nil {
        log.Fatalf("Unable to parse credentials: %v", err)
    }

    // Redirect user to consent page
    config.RedirectURL = "http://localhost:8080/callback"
    authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
    fmt.Printf("Go to the following link in your browser:\n%v\n", authURL)

    // Start HTTP server to receive callback
    http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
        code := r.URL.Query().Get("code")
        token, err := config.Exchange(ctx, code)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Save token for future use
        tokenJSON, _ := json.Marshal(token)
        os.WriteFile("token.json", tokenJSON, 0600)

        // Create Drive client
        client, err := gdrive.NewDriveClientWithToken(ctx, config, token)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Use client
        files, _ := client.ListFiles(ctx)
        fmt.Fprintf(w, "Found %d files", len(files))
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### Service Account (Server Application)

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/abiiranathan/gdrive"
)

func main() {
    ctx := context.Background()

    // Read service account credentials
    credentials, err := os.ReadFile("service-account.json")
    if err != nil {
        log.Fatalf("Unable to read service account file: %v", err)
    }

    // Create client
    client, err := gdrive.NewDriveClientForServiceAccount(ctx, credentials)
    if err != nil {
        log.Fatalf("Unable to create Drive client: %v", err)
    }

    // List files
    files, err := client.ListFiles(ctx)
    if err != nil {
        log.Fatalf("Unable to list files: %v", err)
    }

    fmt.Printf("Found %d files\n", len(files))
    for _, file := range files {
        fmt.Printf("- %s (%s)\n", file.Name, file.FolderPath)
    }
}
```

## Listing Files

### List All Files

```go
files, err := client.ListFiles(ctx)
if err != nil {
    log.Fatal(err)
}

for _, file := range files {
    fmt.Printf("Name: %s\n", file.Name)
    fmt.Printf("ID: %s\n", file.ID)
    fmt.Printf("Size: %d bytes\n", file.Size)
    fmt.Printf("Type: %s\n", file.MimeType)
    fmt.Printf("Path: %s\n", file.FolderPath)
    fmt.Printf("Link: %s\n\n", file.WebViewLink)
}
```

### List Files in Specific Folder

```go
folderID := "1aBc2DeFg3HiJ4KlM5nOp"
files, err := client.ListFilesInFolder(ctx, folderID)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d files in folder\n", len(files))
```

### Filter Files by Type

```go
files, err := client.ListFiles(ctx)
if err != nil {
    log.Fatal(err)
}

// Find all PDFs
var pdfs []gdrive.FileInfo
for _, file := range files {
    if file.MimeType == "application/pdf" {
        pdfs = append(pdfs, file)
    }
}

fmt.Printf("Found %d PDF files\n", len(pdfs))
```

## Uploading Files

### Upload Single File

```go
fileID, err := client.UploadFile(ctx, "/path/to/document.pdf", "Monthly Report.pdf", "")
if err != nil {
    log.Fatalf("Upload failed: %v", err)
}
fmt.Printf("File uploaded with ID: %s\n", fileID)
```

### Upload to Specific Folder

```go
folderID := "1aBc2DeFg3HiJ4KlM5nOp"
fileID, err := client.UploadFile(ctx, "/path/to/file.pdf", "file.pdf", folderID)
if err != nil {
    log.Fatal(err)
}
```

### Batch Upload

```go
files := []string{
    "/docs/report1.pdf",
    "/docs/report2.pdf",
    "/docs/report3.pdf",
}

for _, filePath := range files {
    fileID, err := client.UploadFile(ctx, filePath, "", "")
    if err != nil {
        log.Printf("Failed to upload %s: %v", filePath, err)
        continue
    }
    fmt.Printf("Uploaded %s (ID: %s)\n", filePath, fileID)
}
```

### Upload from Memory

```go
import "bytes"

data := []byte("Hello, Google Drive!")
reader := bytes.NewReader(data)

fileID, err := client.UploadFileFromReader(ctx, reader, "hello.txt", "text/plain", "")
if err != nil {
    log.Fatal(err)
}
```

## Downloading Files

### Download to File

```go
fileID := "1aBc2DeFg3HiJ4KlM5nOp"
outputPath := "/downloads/document.pdf"

bytesWritten, err := client.DownloadFile(ctx, fileID, outputPath)
if err != nil {
    log.Fatalf("Download failed: %v", err)
}
fmt.Printf("Downloaded %d bytes to %s\n", bytesWritten, outputPath)
```

### Download to Memory

```go
import "bytes"

var buf bytes.Buffer
bytesWritten, err := client.StreamFile(ctx, fileID, &buf)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Downloaded %d bytes to memory\n", bytesWritten)
// Use buf.Bytes() to access the data
```

### Batch Download

```go
fileIDs := []string{
    "1aBc2DeFg3HiJ",
    "4KlM5nOp6QrS",
    "7TuV8wXy9ZaB",
}

for i, fileID := range fileIDs {
    outputPath := fmt.Sprintf("/downloads/file%d.bin", i+1)
    _, err := client.DownloadFile(ctx, fileID, outputPath)
    if err != nil {
        log.Printf("Failed to download %s: %v", fileID, err)
        continue
    }
    fmt.Printf("Downloaded file %d\n", i+1)
}
```

## Working with Folders

### Create Folder

```go
folderID, err := client.CreateFolder(ctx, "Project Files", "")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Created folder with ID: %s\n", folderID)
```

### Create Nested Folders

```go
// Create parent folder
parentID, err := client.CreateFolder(ctx, "Projects", "")
if err != nil {
    log.Fatal(err)
}

// Create subfolder
subfolderID, err := client.CreateFolder(ctx, "2024", parentID)
if err != nil {
    log.Fatal(err)
}

// Upload file to subfolder
fileID, err := client.UploadFile(ctx, "/docs/Q1.pdf", "Q1 Report.pdf", subfolderID)
```

### Organize Files

```go
// Create folder structure
foldersData := map[string]string{
    "Documents": "",
    "Images":    "",
    "Videos":    "",
}

folderIDs := make(map[string]string)
for folderName := range foldersData {
    folderID, err := client.CreateFolder(ctx, folderName, "")
    if err != nil {
        log.Printf("Failed to create %s: %v", folderName, err)
        continue
    }
    folderIDs[folderName] = folderID
}

// Upload files to appropriate folders
files := []struct {
    path   string
    folder string
}{
    {"/files/doc1.pdf", "Documents"},
    {"/files/photo.jpg", "Images"},
    {"/files/video.mp4", "Videos"},
}

for _, file := range files {
    folderID := folderIDs[file.folder]
    _, err := client.UploadFile(ctx, file.path, "", folderID)
    if err != nil {
        log.Printf("Failed to upload %s: %v", file.path, err)
    }
}
```

## Google Workspace Documents

### Export Google Doc to PDF

```go
docID := "1aBc2DeFg3HiJ4KlM5nOp"

// Check if it's a Workspace document
isWorkspace, err := client.IsWorkspaceDocument(ctx, docID)
if err != nil {
    log.Fatal(err)
}

if !isWorkspace {
    log.Fatal("Not a Google Workspace document")
}

// Export to PDF
bytesWritten, err := client.ExportWorkspaceDocumentToFile(
    ctx,
    docID,
    "/exports/document.pdf",
    gdrive.ExportFormatPDF,
)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Exported %d bytes\n", bytesWritten)
```

### Export Google Sheet to Excel

```go
sheetID := "1aBc2DeFg3HiJ4KlM5nOp"

bytesWritten, err := client.ExportWorkspaceDocumentToFile(
    ctx,
    sheetID,
    "/exports/spreadsheet.xlsx",
    gdrive.ExportFormatXLSX,
)
if err != nil {
    log.Fatal(err)
}
```

### Get Available Export Formats

```go
docID := "1aBc2DeFg3HiJ4KlM5nOp"

links, err := client.GetExportLinks(ctx, docID)
if err != nil {
    log.Fatal(err)
}

fmt.Println("Available export formats:")
for mimeType, url := range links {
    fmt.Printf("- %s\n", mimeType)
}
```

### Batch Export Documents

```go
docs := []struct {
    id     string
    name   string
    format gdrive.ExportFormat
}{
    {"doc1", "Report", gdrive.ExportFormatPDF},
    {"sheet1", "Data", gdrive.ExportFormatXLSX},
    {"slide1", "Presentation", gdrive.ExportFormatPPTX},
}

for _, doc := range docs {
    outputPath := fmt.Sprintf("/exports/%s.%s", doc.name, getExtension(doc.format))
    _, err := client.ExportWorkspaceDocumentToFile(ctx, doc.id, outputPath, doc.format)
    if err != nil {
        log.Printf("Failed to export %s: %v", doc.name, err)
        continue
    }
    fmt.Printf("Exported %s\n", doc.name)
}

func getExtension(format gdrive.ExportFormat) string {
    switch format {
    case gdrive.ExportFormatPDF:
        return "pdf"
    case gdrive.ExportFormatXLSX:
        return "xlsx"
    case gdrive.ExportFormatPPTX:
        return "pptx"
    default:
        return "bin"
    }
}
```

## Partial Downloads

### Resumable Download

```go
import "os"

fileID := "1aBc2DeFg3HiJ4KlM5nOp"
outputPath := "/downloads/largefile.bin"
chunkSize := int64(1048576) // 1 MB chunks

// Get file size first
files, _ := client.ListFiles(ctx)
var fileSize int64
for _, f := range files {
    if f.ID == fileID {
        fileSize = f.Size
        break
    }
}

// Download in chunks
file, err := os.Create(outputPath)
if err != nil {
    log.Fatal(err)
}
defer file.Close()

var totalDownloaded int64
for offset := int64(0); offset < fileSize; offset += chunkSize {
    endByte := offset + chunkSize - 1
    if endByte >= fileSize {
        endByte = fileSize - 1
    }

    bytesWritten, err := client.PartialStreamFile(ctx, fileID, file, offset, endByte)
    if err != nil {
        log.Printf("Error downloading chunk at offset %d: %v", offset, err)
        break
    }

    totalDownloaded += bytesWritten
    progress := float64(totalDownloaded) / float64(fileSize) * 100
    fmt.Printf("Progress: %.2f%%\r", progress)
}
fmt.Println("\nDownload complete!")
```

### Download Specific Range

```go
import "bytes"

// Download first 1 KB
var buf bytes.Buffer
opts := gdrive.PartialDownloadOptions{
    StartByte: 0,
    EndByte:   1023,
}

bytesWritten, err := client.PartialDownloadFile(ctx, fileID, &buf, opts)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Downloaded first %d bytes\n", bytesWritten)
```

## Web Application Integration

### File Upload Handler

```go
import (
    "net/http"
    "path/filepath"
)

func uploadHandler(w http.ResponseWriter, r *http.Request, client *gdrive.DriveClient) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Parse multipart form
    if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB max
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    file, header, err := r.FormFile("file")
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Get folder ID from form
    folderID := r.FormValue("folder_id")

    // Upload to Google Drive
    ctx := r.Context()
    fileID, err := client.UploadFileFromReader(
        ctx,
        file,
        header.Filename,
        header.Header.Get("Content-Type"),
        folderID,
    )
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    fmt.Fprintf(w, "File uploaded successfully! ID: %s", fileID)
}
```

### File Download Handler

```go
func downloadHandler(w http.ResponseWriter, r *http.Request, client *gdrive.DriveClient) {
    fileID := r.URL.Query().Get("id")
    if fileID == "" {
        http.Error(w, "File ID required", http.StatusBadRequest)
        return
    }

    ctx := r.Context()

    // Get file info
    files, err := client.ListFiles(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    var fileInfo *gdrive.FileInfo
    for _, f := range files {
        if f.ID == fileID {
            fileInfo = &f
            break
        }
    }

    if fileInfo == nil {
        http.Error(w, "File not found", http.StatusNotFound)
        return
    }

    // Set headers
    w.Header().Set("Content-Type", fileInfo.MimeType)
    w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileInfo.Name))
    w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size))

    // Stream file to response
    _, err = client.StreamFile(ctx, fileID, w)
    if err != nil {
        log.Printf("Error streaming file: %v", err)
    }
}
```

### File Browser API

```go
func listFilesHandler(w http.ResponseWriter, r *http.Request, client *gdrive.DriveClient) {
    ctx := r.Context()
    folderID := r.URL.Query().Get("folder")

    var files []gdrive.FileInfo
    var err error

    if folderID == "" {
        files, err = client.ListFiles(ctx)
    } else {
        files, err = client.ListFilesInFolder(ctx, folderID)
    }

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(files)
}
```

## Error Handling

### Graceful Error Handling

```go
func safeListFiles(client *gdrive.DriveClient) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    files, err := client.ListFiles(ctx)
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            log.Println("Request timeout")
        } else {
            log.Printf("API error: %v", err)
        }
        return
    }

    fmt.Printf("Found %d files\n", len(files))
}
```

### Retry Logic

```go
import "time"

func uploadWithRetry(client *gdrive.DriveClient, filePath string, maxRetries int) (string, error) {
    var fileID string
    var err error

    for i := 0; i < maxRetries; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
        fileID, err = client.UploadFile(ctx, filePath, "", "")
        cancel()

        if err == nil {
            return fileID, nil
        }

        log.Printf("Attempt %d failed: %v", i+1, err)
        if i < maxRetries-1 {
            time.Sleep(time.Second * time.Duration(1<<uint(i))) // Exponential backoff
        }
    }

    return "", fmt.Errorf("failed after %d attempts: %w", maxRetries, err)
}
```

### Checking File Existence

```go
func fileExists(client *gdrive.DriveClient, fileName string) (bool, string, error) {
    ctx := context.Background()
    files, err := client.ListFiles(ctx)
    if err != nil {
        return false, "", err
    }

    for _, file := range files {
        if file.Name == fileName {
            return true, file.ID, nil
        }
    }

    return false, "", nil
}
```
