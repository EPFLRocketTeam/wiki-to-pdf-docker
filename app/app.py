from flask import Flask, request, jsonify, render_template, send_file, session, send_from_directory
from datetime import datetime
from urllib.parse import urlparse
import requests
import re
import subprocess
import uuid
import redis
import json
import os
import shutil
import tempfile
import zipfile
from flask_cors import CORS
from markdown_converter import MarkdownConverter, ConversionError
from page_data_manager import PageDataManager, PageMetadata


app = Flask(__name__)
CORS(app, origins=["https://rocket-team.epfl.ch"])
converter = MarkdownConverter()
page_manager = PageDataManager()

# Ensure JWT token is stored securely
#JWT_TOKEN = os.getenv("JWT_TOKEN")

class WikiPage:
    def __init__(self, path, locale):
        self.path = path
        self.locale = locale

def add_draft_to_documentclass(tex_file_path):
    """
    Adds the `draft` option to the `\documentclass` line in a LaTeX file.
    """
    with open(tex_file_path, 'r') as file:
        lines = file.readlines()

    modified_lines = []
    for line in lines:
        if line.strip().startswith(r'\documentclass'):
            if '[' in line:
                # Add `draft` to existing options
                line = line.replace('[', '[draft,')
            else:
                # Add `[draft]` if no options exist
                line = line.replace(r'\documentclass{', r'\documentclass[draft]{')
        modified_lines.append(line)

    with open(tex_file_path, 'w') as file:
        file.writelines(modified_lines)

def remove_draft_from_documentclass(tex_file_path):
    """
    Removes the `draft` option from the `\documentclass` line in a LaTeX file.
    """
    with open(tex_file_path, 'r') as file:
        lines = file.readlines()

    modified_lines = []
    for line in lines:
        if line.strip().startswith(r'\documentclass') and 'draft' in line:
            # Remove the `draft` option
            line = line.replace('draft,', '').replace(',draft', '').replace('[draft]', '')
        modified_lines.append(line)

    with open(tex_file_path, 'w') as file:
        file.writelines(modified_lines)

def compile_latex_with_draft(tex_file_path):
    """
    Compiles the LaTeX file twice: first with the `draft` option, then without.
    """
    # Add `draft` to \documentclass
    add_draft_to_documentclass(tex_file_path)

    # First compilation with `draft`
    result = subprocess.run([
        'lualatex', '-shell-escape', '-output-directory', '/tmp', tex_file_path
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    if result.returncode == 0:
        print("First compilation (with draft) succeeded.")
    else:
        print("First compilation (with draft) failed.")
        print(result.stderr.decode())
        return result # Stop if the first run fails

    # Remove `draft` from \documentclass
    remove_draft_from_documentclass(tex_file_path)

    # Second compilation without `draft`
    result = subprocess.run([
        'lualatex', '-shell-escape', '-output-directory', '/tmp', tex_file_path
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    if result.returncode == 0:
        print("Second compilation (without draft) succeeded.")
    else:
        print("Second compilation (without draft) failed.")
        print(result.stderr.decode())
    return result

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
    #result = subprocess.run([
        #'lualatex', '-shell-escape', '-output-directory', '/tmp', tex_file_path
    #], )#stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    #print(result.returncode)
    result = compile_latex_with_draft(tex_file_path)

    if result.returncode != 0:
        # Handle compilation errors by returning the error message
        return {
            "error": "Failed to compile PDF",
            "message": "----------StdErr----------\n" + result.stderr.decode() + "\n----------StdOut----------\n" + result.stdout.decode()
        }, 500

    return send_file(pdf_file_path, mimetype='application/pdf', as_attachment=True)

# Connect to Redis
redis_host = os.getenv('REDIS_HOST', 'some-redis') # Default to localhost for local dev
redis_port = int(os.getenv('REDIS_PORT', 6379)) # Default to 6379
redis_client = redis.StrictRedis(host=redis_host, port=redis_port, db=0, decode_responses=True)

@app.route('/store', methods=['POST'])
def store_data():
    data = request.json
    if not data:
        return jsonify({"error": "Invalid data"}), 400

    session_id = str(uuid.uuid4())  # Generate unique ID
    redis_client.set(session_id, json.dumps(data))  # Store in Redis (serialize data)

    return jsonify({"session_id": session_id})

@app.route('/edit')
def edit_page():
    session_id = request.args.get("session_id")
    if not session_id or not redis_client.exists(session_id):
        return "Invalid session", 400

    data = redis_client.get(session_id)
    return render_template("edit.html", data=data)  # Pass to frontend

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
            'documentId': data.get('documentId', ''),
            'lineNumbers': data.get('lineNumbersEnabled', False)
        }

        if not markdown_content:
            return jsonify({'error': 'No markdown content provided'}), 400

        latex_content = converter.convert_to_latex(
            filtered_markdown_content,
            template=template,
            metadata=metadata
        )
        
        # 1. Create a unique temporary directory
        temp_dir_name = str(uuid.uuid4())
        project_temp_path = os.path.join(tempfile.gettempdir(), temp_dir_name)
        os.makedirs(project_temp_path)

        main_tex_path = os.path.join(project_temp_path, 'main.tex')
        with open(main_tex_path, 'w') as f:
            f.write(latex_content)

        # 2. **CRITICAL STEP:** Identify and copy/generate all external assets
        #    This is the most complex part and depends on how your markdown converter
        #    handles images/drawio, and where those files are stored in your Wiki.js.
        #    You would parse 'latex_content' to find image/include paths,
        #    then fetch those files from Wiki.js (if remote) or copy from local storage
        #    into 'project_temp_path'.

        #    Example (placeholder for your logic):
        #    For each image_path in latex_content:
        #        source_image_path = resolve_wiki_image_path(image_path)
        #        if source_image_path:
        #            shutil.copy(source_image_path, os.path.join(project_temp_path, os.path.basename(image_path)))

        # 3. Create a ZIP archive
        zip_file_name = f"{temp_dir_name}.zip"
        zip_file_path = os.path.join(tempfile.gettempdir(), zip_file_name)

        # Create the zip from the contents of project_temp_path
        # The base directory will be temp_dir_name, which is fine as Overleaf unpacks it
        shutil.make_archive(
            os.path.join(tempfile.gettempdir(), temp_dir_name), # Base name for the archive
            'zip',                                              # Archive format
            project_temp_path                                   # Directory to archive
        )
        # Rename it to .zip if make_archive doesn't add it (it usually does for 'zip')
        if not zip_file_path.endswith('.zip'):
            zip_file_path = zip_file_path + '.zip'


        # 4. Store the path to the ZIP in Redis, associated with a session_id
        session_id = str(uuid.uuid4())
        redis_client.set(f"zip_project:{session_id}", zip_file_path) # Store path to the zip
        redis_client.expire(f"zip_project:{session_id}", 300) # Expire in 5 minutes

        # 5. Clean up the temporary directory after zipping (optional, but good)
        shutil.rmtree(project_temp_path)

        return jsonify({
            'latex': latex_content,
            'status': 'success',
            'session_id': session_id
        })

    except ConversionError as e:
        return jsonify({'error': str(e)}), 500
    except Exception as e:
        return jsonify({'error': f'Unexpected error: {str(e)}'}), 500

@app.route('/serve-zip-project/<session_id>', methods=['GET'])
def serve_zip_project_for_overleaf(session_id):
    zip_file_path = redis_client.get(f"zip_project:{session_id}")

    if not zip_file_path or not os.path.exists(zip_file_path):
        return "Project ZIP file not found or expired.", 404

    directory = os.path.dirname(zip_file_path)
    filename = os.path.basename(zip_file_path)

    # Use send_from_directory to correctly serve the file
    # Overleaf will look for a ZIP, so mimetype is important
    response = send_from_directory(directory, filename, as_attachment=True, mimetype='application/zip')

    # Clean up the zip file immediately after serving it
    @response.call_on_close
    def cleanup_zip():
        try:
            os.remove(zip_file_path)
            print(f"Cleaned up zip file: {zip_file_path}")
        except Exception as e:
            print(f"Error cleaning up zip file {zip_file_path}: {e}")

    return response

if __name__ == '__main__':
    app.run(debug=True)
