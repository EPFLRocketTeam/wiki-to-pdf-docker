from flask import Flask, request, jsonify, render_template

app = Flask(__name__)

# Route to display the form where users can input parameters
@app.route('/')
def index():
    return render_template('index.html')

# Route to handle the form submission
@app.route('/submit', methods=['POST'])
def submit():
    wiki_url = request.form.get('wiki-url')
    page_path = request.form.get('page-path')
    page_locale = request.form.get('page-locale')
    login_type = request.form.get('login-type')
    username = request.form.get('username')
    password = request.form.get('password')
    access_token = request.form.get('access-token')
    
    markup_type = request.form.get('markup-type')
    lua_filter = request.form.get('lua-filter')
    template = request.form.get('template')
    
    compile_on_server = request.form.get('compile-on-server') == 'on'
    compile_on_browser = request.form.get('compile-on-browser') == 'on'

    # For now, just returning the received parameters
    response = {
        'wiki-url': wiki_url,
        'page-path': page_path,
        'page-locale': page_locale,
        'login-type': login_type,
        'username': username,
        'password': password,
        'access-token': access_token,
        'markup-type': markup_type,
        'lua-filter': lua_filter,
        'template': template,
        'compile-on-server': compile_on_server,
        'compile-on-browser': compile_on_browser,
    }
    
    return jsonify(response)

if __name__ == '__main__':
    app.run(debug=True)
