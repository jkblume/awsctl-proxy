## Guidelines

### Error Handling in Go
- When creating a NEW error (not wrapping), the error message should be like: "failed to do something"
  Example: `return fmt.Errorf("failed to parse configuration")`
  
- When WRAPPING an existing error, just describe the action being performed without "failed to":
  Example: `return fmt.Errorf("parse configuration: %w", err)`
  This creates a clear error chain: "parse configuration: open config.json: no such file or directory"
  
- The final error chain reads naturally: each level adds context about what it was trying to do
- Use errors.Is and errors.As to check for specific errors

Example of proper error handling:
```go
// Creating a new error
if user == nil {
    return fmt.Errorf("user cannot be nil")
}

// Wrapping an error - just state the action
config, err := loadConfig()
if err != nil {
    return fmt.Errorf("load config: %w", err)
}

// Multiple levels of wrapping
data, err := db.Query()
if err != nil {
    return fmt.Errorf("query user data: %w", err)
}
// This might produce: "query user data: execute SQL: connection refused"
```
