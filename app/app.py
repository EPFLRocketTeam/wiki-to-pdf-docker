from flask import Flask, request, jsonify, render_template, send_file
from datetime import datetime
from urllib.parse import urlparse
import requests
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
    email = request.json.get('email')
    password = request.json.get('password')
    
    if not urls:
        return jsonify({'error': 'No URLs provided'}), 400
    if not email or not password:
        return jsonify({'error': 'Email and password are required'}), 400
    
    # Step 1: Authenticate to get JWT token
    auth_url = "https://rocket-team.epfl.ch/graphql"
    auth_query = {
        'query': """
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
        """ % (email, password)
    }
    
    auth_response = requests.post(auth_url, json=auth_query)
    if auth_response.status_code != 200 or 'errors' in auth_response.json():
        return jsonify({'error': 'Authentication failed'}), 401
    
    jwt_token = auth_response.json()['data']['authentication']['login']['jwt']
    
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
            markdown_content,
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
