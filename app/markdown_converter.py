from typing import Dict, Optional
import pypandoc
import os
import tempfile
import yaml

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
                    yaml.dump(metadata, f, allow_unicode=True, sort_keys=False)
                    f.write(f"assetsDirectory: \"{os.path.dirname(os.path.realpath(__file__))}/latex_templates/template_images\"\n")
            
            # Set up conversion options
            extra_args = [
                "--standalone",
                "--from", "markdown",
                "--to", "latex",
                "--lua-filter", "/app/ImageLuaFilter.lua",
                "--variable", "code-block-environment=minted"
            ]
            
            # Use a single base template and supply per-template variables
            base_template_path = f"{os.path.dirname(os.path.realpath(__file__))}/latex_templates/base.tex"
            # Mapping of template -> variables (backgroundImage, rheadImage, footerText)
            template_vars = {
                "default": {},
                "generic": {"backgroundImage": "ert_title-page.png"},
                "competition": {"backgroundImage": "c_title-page.png", "rheadImage": "c_patch.png"},
                "hyperion": {"backgroundImage": "h_title-page.png", "rheadImage": "h_patch.png"},
                "icarus": {"backgroundImage": "i_title-page.png", "rheadImage": "i_patch.png"},
                "management": {"backgroundImage": "m_title-page.png"},
                "space-race": {"backgroundImage": "s_title-page.png", "rheadImage": "s_patch.png"},
            }

            # Use base template if it exists
            if os.path.exists(base_template_path):
                extra_args.extend(["--template", base_template_path])
                # Add variables for the requested template
                vars_for_template = template_vars.get(template, {})
                for k, v in vars_for_template.items():
                    extra_args.extend(["--variable", f"{k}={v}"])
            
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
