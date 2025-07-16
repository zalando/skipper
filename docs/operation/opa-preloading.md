# OPA Pre-loading Feature

## Overview

This feature implements issue #3526 by moving OPA instance startup and bundle downloading out of filter creation to make route processing faster and more reliable.

## Problem

Previously, OPA filters created instances synchronously during filter creation, which:
- Blocked route processing during bundle downloads
- Could cause timeouts with large bundles or slow networks
- Violated the assumption that filter creation should be fast

## Solution

When enabled via the `--enable-open-policy-agent-preloading` flag, OPA instances are pre-loaded during route processing instead of filter creation:

1. **Route PreProcessor**: Scans routes for OPA filter usage and pre-loads instances
2. **Non-blocking Filter Creation**: Filters only lookup ready instances, failing fast if not available
3. **Smart Loading Strategy**: Parallel loading on startup, sequential on route updates

## Usage

### Enable the Feature

```bash
skipper --enable-open-policy-agent --enable-open-policy-agent-preloading
```

### Configuration

- `--enable-open-policy-agent-preloading`: Enable the pre-loading behavior (default: false)
- All existing OPA configuration flags remain the same

### Behavior Changes

#### With Pre-loading Disabled (Default)
- **Filter Creation**: Synchronous, blocks until OPA instance is ready
- **Route Processing**: Fast, instances already created
- **Backward Compatibility**: 100% compatible with existing behavior

#### With Pre-loading Enabled
- **Route Processing**: Pre-loads OPA instances in background
- **Filter Creation**: Fast lookup, fails immediately if instance not ready
- **Startup**: Parallel instance creation for faster startup
- **Updates**: Sequential instance creation to avoid CPU spikes

## Implementation Details

### New Components

1. **PreProcessor** (`preprocessor.go`):
   - Scans routes for OPA filter requirements
   - Triggers instance pre-loading
   - Handles parallel vs sequential loading

2. **Registry Enhancements** (`openpolicyagent.go`):
   - `GetOrStartInstance()`: Non-blocking instance retrieval
   - Async instance creation with status tracking
   - Backward compatibility with existing `NewOpenPolicyAgentInstance()`

3. **Filter Modifications**:
   - Minimal changes to use non-blocking lookup
   - Graceful fallback to synchronous behavior when pre-loading disabled

### Error Handling

- Routes with unavailable OPA instances are marked as invalid
- Clear error messages indicate when instances are not ready
- Proper cleanup of failed instance creation attempts

## Migration

### Gradual Rollout
1. Deploy with flag disabled (default) - no behavior change
2. Enable flag in staging environment for testing  
3. Enable in production after validation

### Rollback
- Simply disable the flag to return to previous behavior
- No data migration or configuration changes needed

## Testing

The implementation includes comprehensive tests for:
- PreProcessor functionality
- Registry behavior with/without pre-loading
- Filter creation in both modes
- Error conditions and cleanup

## Performance Impact

### Expected Improvements
- **Faster Route Updates**: No blocking on bundle downloads
- **Faster Startup**: Parallel OPA instance creation
- **Better Reliability**: Failed instances don't block other routes

### Resource Usage
- **Memory**: Minimal increase for async tracking structures
- **CPU**: More efficient with sequential loading during updates
- **Network**: Same bundle download behavior, better orchestrated

## Monitoring

Existing OPA metrics continue to work. Consider monitoring:
- Route processing times
- OPA instance creation success/failure rates
- Bundle download times

## Future Enhancements

This foundation enables future optimizations:
- Bundle sharing across multiple routes
- Smart caching of bundle configurations
- Health monitoring integration
- Advanced pre-loading policies
