# Setagaya Documentation Links Configuration

The Setagaya platform includes configurable documentation links that appear in the user interface to help users navigate
to relevant documentation and guides.

## Configuration Parameters

### `project_home`

- **Purpose**: Link to main project documentation or wiki
- **Default**: `https://docs.example.com/setagaya/project-home`
- **Usage**: Displayed in the UI as a "Project Home" link

### `upload_file_help`

- **Purpose**: Link to file upload instructions and guidelines
- **Default**: `https://docs.example.com/setagaya/file-upload-guide`
- **Usage**: Displayed as help text or link during file upload operations

## Customization

### Helm Deployment

Update your `values.yaml` file:

```yaml
runtime:
  project_home: 'https://your-organization.com/setagaya/docs'
  upload_file_help: 'https://your-organization.com/setagaya/upload-guide'
```

### Direct Configuration

Update your `config.json` file:

```json
{
  "project_home": "https://your-organization.com/setagaya/docs",
  "upload_file_help": "https://your-organization.com/setagaya/upload-guide"
}
```

### Local Development

For local development, the template configuration (`config_tmpl.json`) includes example URLs that can be customized for
your organization's documentation structure.

## Best Practices

1. **Use HTTPS**: Always use secure URLs for documentation links
2. **Organization-Specific**: Customize URLs to point to your organization's documentation
3. **Accessible**: Ensure linked documentation is accessible to all Setagaya users
4. **Current**: Keep documentation links updated when moving or restructuring docs
5. **Descriptive**: Use clear, descriptive URLs that indicate the content purpose

## Migration from Hardcoded URLs

Previous versions included hardcoded references to specific documentation systems. These have been replaced with
configurable parameters to support:

- **Multi-organization deployment**: Different organizations can use their own documentation
- **Flexible documentation systems**: Support for any documentation platform
- **Easy maintenance**: URLs can be updated without code changes

The default placeholder URLs (`docs.example.com`) should be replaced with your actual documentation URLs before
deployment.
