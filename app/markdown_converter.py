from typing import Dict, Optional
import pypandoc
import os
import tempfile

class MarkdownConverter:
    def __init__(self):
        # Ensure pypandoc is properly initialized
        pypandoc.download_pandoc()
        
    def convert_to_latex(
        self, 
        markdown: str, 
        template: str = "default",
        metadata: Optional[Dict] = None
    ) -> str:
        """
        Convert markdown content to LaTeX using pypandoc.
        
        Args:
            markdown (str): The markdown content to convert
            template (str): The template to use (default, competition, etc.)
            metadata (Dict): Optional metadata like author, date, title
            
        Returns:
            str: The converted LaTeX content
        """
        # Create temporary directory for any auxiliary files
        with tempfile.TemporaryDirectory() as temp_dir:
            # Create metadata file if provided
            metadata_file = None
            if metadata:
                metadata_file = os.path.join(temp_dir, "metadata.yaml")
                with open(metadata_file, "w") as f:
                    for key, value in metadata.items():
                        f.write(f"{key}: {value}\n")
                    f.write(f"assetsDirectory: {os.path.dirname(os.path.realpath(__file__))}/latex_templates/template_images\n")
            
            # Set up conversion options
            extra_args = [
                "--standalone",
                "--from", "markdown",
                "--to", "latex"
            ]
            
            # Add template if specified
            if template != "default":
                template_path = f"{os.path.dirname(os.path.realpath(__file__))}/latex_templates/{template}.tex"
                print(template_path, os.path.exists(template_path))
                if os.path.exists(template_path):
                    extra_args.extend(["--template", template_path])
            
            # Add metadata file if it exists
            if metadata_file:
                extra_args.extend(["--metadata-file", metadata_file])
            
            try:
                # Perform the conversion
                latex_content = pypandoc.convert_text(
                    markdown,
                    "latex",
                    format="markdown",
                    extra_args=extra_args
                )
                return latex_content
                
            except Exception as e:
                raise ConversionError(f"Failed to convert markdown to LaTeX: {str(e)}")

class ConversionError(Exception):
    """Custom exception for conversion errors"""
    pass
