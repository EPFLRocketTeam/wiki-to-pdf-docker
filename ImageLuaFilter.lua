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
    local temp_svg = os.tmpname() .. ".svg"
    write_file(temp_svg, svg_content)
    os.execute("drawio --export --format pdf --output " .. output_pdf .. " " .. temp_svg .. " > /dev/null 2>&1")
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
  local width, height

  -- Parse the "=widthxheight" syntax at the end of the source
  local src_clean, dimensions = src:match("^(.-)%s*=%s*(.-)x(.-)$")
  if dimensions then
    src = src_clean
    -- Match numeric or percentage width and height
    width = dimensions:match("^(%d+%%?)$")
    height = dimensions:match("^(%d+%%?)$")
  end

  -- Prepend the fixed base path
  src = "/home/jordan/ert_wiki/" .. src

  -- Add width and height to attributes if they exist
  if width then
    elem.attributes["width"] = width
  end
  if height then
    elem.attributes["height"] = height
  end

  -- Preserve alignment or other classes (e.g., .align-center)
  if elem.classes then
    elem.attributes["class"] = table.concat(elem.classes, " ")
  end

  elem.src = src -- Update the source
  return elem
end
