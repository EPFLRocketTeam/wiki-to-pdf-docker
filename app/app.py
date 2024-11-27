from flask import Flask, request, jsonify, render_template, send_file
from datetime import datetime
from urllib.parse import urlparse
import requests
import re
import os
import io
import subprocess
from markdown_converter import MarkdownConverter, ConversionError
from page_data_manager import PageDataManager, PageMetadata

app = Flask(__name__)
converter = MarkdownConverter()
page_manager = PageDataManager()

# Ensure JWT token is stored securely
#JWT_TOKEN = os.getenv("JWT_TOKEN")

class WikiPage:
    def __init__(self, path, locale):
        self.path = path
        self.locale = locale

def remove_backtick_content(text):
    """
    Removes content enclosed within triple backticks, including the backticks themselves.
    """
    # Pattern to match content within triple backticks
    pattern = r'```.*?```'
    # Remove the content
    cleaned_text = re.sub(pattern, '', text, flags=re.DOTALL)
    return cleaned_text

def remove_links_list_tag(text):
    """
    Removes the tag '{.links-list}' from the text.
    """
    pattern = r'\{\.links-list\}'
    cleaned_text = re.sub(pattern, '', text)
    return cleaned_text

def add_blank_line_after_titles(text):
    """
    Adds a blank line after each title starting with '##', except for '## table {.tabset}',
    and only if the title is immediately followed by a specific table pattern.
    """
    lines = text.split('\n')
    result_lines = []
    i = 0
    while i < len(lines):
        line = lines[i]
        result_lines.append(line)
        
        # Check if the line is a title starting with '##'
        if re.match(r'^##\s', line):
            # Skip '## table {.tabset}' titles
            if not re.match(r'^##\s+table\s+\{\.tabset\}', line):
                # Check if the next two lines exist
                if i + 2 < len(lines):
                    next_line = lines[i + 1]
                    next_next_line = lines[i + 2]
                    # Check for the specific table pattern
                    if re.match(r'^\|.*\|$', next_line.strip()) and re.match(r'^\|[-:]+.*\|$', next_next_line.strip()):
                        # Add a blank line after the title
                        result_lines.append('')
        i += 1
    # Reconstruct the text
    updated_text = '\n'.join(result_lines)
    return updated_text

def filter_text(text):
    #text_without_diagrams= remove_backtick_content(text)
    text_without_tags=remove_links_list_tag(text)
    text_correct_formated_tabs = add_blank_line_after_titles(text_without_tags)
    return text_correct_formated_tabs

def fetch_wiki_contents(paths: list, locales: list, url: str, jwt_token: str) -> list:
    """Fetch content from Wiki for multiple pages using GraphQL."""
    headers = {
        "Authorization": f"Bearer {jwt_token}",
        "Content-Type": "application/json"
    }
    
    contents = []
    for path, locale in zip(paths, locales):
        query = f"""{{
            pages {{
                singleByPath(path: "{path}", locale: "{locale}") {{
                    path
                    title
                    createdAt
                    updatedAt
                    authorName
                    content
                }}
            }}
        }}"""
        
        response = requests.post(url, headers=headers, json={'query': query})
        
        if response.status_code == 200:
            data = response.json()
            if 'errors' in data:
                contents.append({'path': path, 'error': str(data['errors'])})
            else:
                contents.append(data['data']['pages']['singleByPath'])
        else:
            contents.append({'path': path, 'error': f'HTTP Error: {response.status_code}'})
    
    
    return contents

def parse_rocket_urls(urls: list) -> list:
    """Parse URLs to extract paths and locales, taking into account locales in URL structure."""
    SUPPORTED_LANGUAGES = {'en', 'fr', 'de', 'it'}
    parsed_pages = []

    for url in urls:
        parsed_url = urlparse(url)
        path_segments = [seg for seg in parsed_url.path.split('/') if seg]
        
        # Determine locale
        locale = 'en'
        if path_segments and path_segments[0] in SUPPORTED_LANGUAGES:
            locale = path_segments[0]
            path_segments = path_segments[1:]
        
        # Join the remaining path segments to get the full path
        path = '/'.join(path_segments) if path_segments else 'home'
        
        # Store path and locale
        parsed_pages.append(WikiPage(path=path, locale=locale))
    
    return parsed_pages

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/how-to-get-access-token')
def how_to_get_access_token():
    return render_template('how_to_get_access_token.html')

@app.route('/fetch', methods=['POST'])
def fetch_content():
    urls = request.json.get('urls', [])
    auth_url = request.json.get('graphql_url')
    jwt_token = request.json.get('token')
    
    # Step 2: Fetch content with JWT token
    parsed_data = parse_rocket_urls(urls)
    paths = [page.path for page in parsed_data]
    locales = [page.locale for page in parsed_data]
    
    try:
        content_data = fetch_wiki_contents(paths, locales, auth_url, jwt_token)
        return jsonify(content_data)
    except Exception as e:
        return jsonify({'error': f"Failed to fetch content: {str(e)}"}), 500

@app.route('/convert', methods=['POST'])
def convert_markdown():
    try:
        data = request.json
        markdown_content = data.get('markdown')
        filtered_markdown_content= filter_text(markdown_content) #filter the content to convert it properly to latex
        template = data.get('template', 'default')
        metadata = {
            'author': data.get('author', ''),
            'date': data.get('date', datetime.now().strftime('%Y-%m-%d')),
            'title': data.get('title', ''),
            'documentId': data.get('documentId', '')
        }
        
        if not markdown_content:
            return jsonify({'error': 'No markdown content provided'}), 400
            
        latex_content = converter.convert_to_latex(
            filtered_markdown_content,
            template=template,
            metadata=metadata
        )
        
        return jsonify({
            'latex': latex_content,
            'status': 'success'
        })
        
    except ConversionError as e:
        return jsonify({'error': str(e)}), 500
    except Exception as e:
        return jsonify({'error': f'Unexpected error: {str(e)}'}), 500

@app.route('/get-access-token', methods=['POST'])
def get_access_token():
    try:
        # Parse incoming JSON payload
        data = request.get_json()
        username = data['username']
        password = data['password']
        endpoint_url = data['endpointUrl']

        # GraphQL mutation for authentication
        graphql_query = {
            "query": """
                mutation {
                    authentication {
                        login(
                            username: "%s",
                            password: "%s",
                            strategy: "local"
                        ) {
                            jwt
                        }
                    }
                }
            """ % (username, password)  # Using string interpolation for simplicity
        }

        # Send GraphQL request
        response = requests.post(endpoint_url, json=graphql_query)
        response.raise_for_status()
        graphql_data = response.json()

        # Extract the JWT token from the GraphQL response
        jwt_token = graphql_data['data']['authentication']['login']['jwt']
        return jsonify({"token": jwt_token})

    except requests.exceptions.RequestException as e:
        return jsonify({"error": str(e)}), 500
    except KeyError:
        return jsonify({"error": "Invalid GraphQL response"}), 500

@app.route('/generate-pdf', methods=['POST'])
def generate_pdf():
    latex_code = request.json.get('latex_code')
    title = request.json.get('title', 'document')
    
    # Save the LaTeX code to a temporary .tex file
    tex_file_path = '/tmp/document.tex'
    pdf_file_path = '/tmp/document.pdf'
    
    with open(tex_file_path, 'w') as f:
        f.write(latex_code)
    
    # Compile the LaTeX file to PDF using lualatex with -shell-escape
    result = subprocess.run([
        'lualatex', '-shell-escape', '-output-directory', '/tmp', tex_file_path
    ], )#stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    print(result.returncode)
    
    if result.returncode != 0:
        # Handle compilation errors by returning the error message
        return {
            "error": "Failed to compile PDF",
            "message": result.stderr.decode()
        }, 500

    return send_file(pdf_file_path, mimetype='application/pdf', as_attachment=True)

if __name__ == '__main__':
    app.run(debug=True)
