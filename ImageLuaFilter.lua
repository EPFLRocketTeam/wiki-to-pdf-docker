local pandoc = require 'pandoc'
local system = require 'pandoc.system'
-- Requires the draw.io CLI to be installed (e.g., `draw.io` binary in your PATH)

-- local base64 = require("mime") -- LuaSocket's MIME module for base64 decoding
local function decode_base64(input)
  local b = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  input = string.gsub(input, '[^' .. b .. '=]', '')
  return (input:gsub('.', function(x)
      if x == '=' then return '' end
      local r, f = '', (b:find(x) - 1)
      for i = 6, 1, -1 do r = r .. (f % 2 ^ i - f % 2 ^ (i - 1) > 0 and '1' or '0') end
      return r
    end):gsub('%d%d%d?%d?%d?%d?%d?%d?', function(x)
      if (#x ~= 8) then return '' end
      local c = 0
      for i = 1, 8 do c = c + (x:sub(i, i) == '1' and 2 ^ (8 - i) or 0) end
      return string.char(c)
    end))
end

-- Helper to write and read temporary files
local function write_file(filename, content)
    local file = io.open(filename, "wb")
    file:write(content)
    file:close()
end

local function read_file(filename)
    local file = io.open(filename, "rb")
    local content = file:read("*all")
    file:close()
    return content
end

-- Decode Base64 content to SVG
local function decode_base64_to_svg(base64_content)
    return decode_base64(base64_content)
end

-- Convert SVG to PDF using draw.io CLI
local function convert_svg_to_pdf(svg_content, output_pdf)
  -- Step 1: Create a temporary SVG file
  local temp_svg = os.tmpname() .. ".svg"
  write_file(temp_svg, svg_content)

  -- Step 2: Use Inkscape to convert SVG to PDF
  os.execute("inkscape --export-type=pdf --export-area-drawing --export-filename=" .. output_pdf .. " " .. temp_svg .. " > /dev/null 2>&1")

  -- Step 3: Remove temporary SVG file
  os.remove(temp_svg)
end

function CodeBlock(block)
  if block.classes[1] == "plantuml" then
    local uml_file = os.tmpname() .. ".puml"
    local file = io.open(uml_file, "w")
    file:write(block.text)
    file:close()

    -- Generate a PDF instead of PNG
    os.execute("plantuml -tpdf " .. uml_file)

    -- Return LaTeX code to include the PDF
    return pandoc.RawBlock("latex", "\\includegraphics[width=\\linewidth,keepaspectratio]{" .. uml_file:gsub('%.puml', '.pdf') .. "}")
  elseif block.classes:includes("diagram") then
    -- Decode the SVG content from the block
    local svg_content = decode_base64_to_svg(block.text)

    -- Output PDF path
    local temp_pdf = os.tmpname() .. ".pdf"

    -- Convert SVG to PDF
    convert_svg_to_pdf(svg_content, temp_pdf)

    -- Replace with LaTeX code to include the PDF
    -- Limit the width and height to avoid overflow
    local latex_code = string.format("\\includegraphics[width=\\linewidth,keepaspectratio]{%s}", temp_pdf)
    return pandoc.RawBlock("latex", latex_code)
  end
end

function Image(elem)
  local src = elem.src

  -- Remove any trailing extra syntax (e.g. " =600x" or " =600x300")
  src = src:gsub("%s*=%s*%d+x%d*$", "")

  -- Decode any URL-encoded spaces, then trim any leading/trailing whitespace
  src = src:gsub("%%20", " "):gsub("^%s*(.-)%s*$", "%1")

  -- Remove any leading slashes to avoid double slashes when prepending the base path
  src = src:gsub("^%s*/+", "")

  src = "/app/ert_wiki/" .. src

  local latex_code = string.format("\\includegraphics[width=\\linewidth, height=\\textheight, keepaspectratio]{%s}", src)
  return pandoc.RawInline("latex", latex_code)
end
