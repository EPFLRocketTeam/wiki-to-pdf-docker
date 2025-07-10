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

local box_styles = {
  ["is-info"] = {
    bg_color = "wikiInfoBg",
    border_color = "wikiInfoBorder",
    icon_latex = "\\textcolor{wikiInfoIcon}{\\faInfoCircle}\\ "
  },
  ["is-success"] = {
    bg_color = "wikiSuccessBg",
    border_color = "wikiSuccessBorder",
    icon_latex = "\\textcolor{wikiSuccessIcon}{\\faCheckCircle}\\ "
  },
  ["is-warning"] = {
    bg_color = "wikiWarningBg",
    border_color = "wikiWarningBorder",
    icon_latex = "\\textcolor{wikiWarningIcon}{\\faExclamationTriangle}\\ "
  },
  ["is-error"] = {
    bg_color = "wikiErrorBg",
    border_color = "wikiErrorBorder",
    icon_latex = "\\textcolor{wikiErrorIcon}{\\faTimesCircle}\\ "
  },
  ["default_blockquote_style"] = {
    bg_color = "wikiDefaultQuoteBg",
    border_color = "wikiDefaultQuoteBorder",
    icon_latex = "\\textcolor{wikiDefaultQuoteIcon}{\\faQuoteLeft}\\ "
  },
  ["links-list"] = {
    bg_color = "white",
    border_color = "lightgray",
    icon_latex = "", -- No icon for a list
    custom_tcb_options = {
      "boxsep=5pt",
      "arc=4pt",
      "boxrule=0.5pt",
      "leftrule=2pt",
      "rightrule=0pt",
      "toprule=0pt",
      "bottomrule=0pt",
      "sharp corners",
      "nobeforeafter",
      "breakable"
    }
  },
}

local base_tcb_options = {
  "boxsep=5pt",
  "arc=4pt",
  "boxrule=1pt",
  "leftrule=5pt",
  "rightrule=0pt",
  "toprule=0pt",
  "bottomrule=0pt",
  "sharp corners",
  "nobeforeafter",
  "breakable"
}

function BlockQuote(el)
  local found_style_key = "default_blockquote_style"
  local class_detected_and_processed = false

  -- Step 1: Check for standard block-level attributes (unlikely given user's input, but good practice)
  if el.attributes and el.attributes.classes then
    for _, class_name in ipairs(el.attributes.classes) do
      if box_styles[class_name] then
        found_style_key = class_name
        class_detected_and_processed = true
        break
      end
    end
  end

  -- Step 2: If no standard block class found, scan the content for the literal `{.class}` string pattern
  if not class_detected_and_processed and #el.content > 0 then
    local first_block = el.content[1] -- Get the first block (e.g., a Paragraph)

    -- Assuming the class marker is at the end of the text in the LAST inline element of the first paragraph
    if first_block.tag == "Para" and #first_block.content > 0 then
      local last_inline = first_block.content[#first_block.content] -- Get the last inline element

      if last_inline.tag == "Str" then -- Check if it's a string element
        local original_text = last_inline.text
        local detected_class = nil
        local suffix_to_remove = nil

        -- Iterate through known classes to find a match as a suffix in the string
        for class_name, _ in pairs(box_styles) do
          if class_name ~= "default_blockquote_style" then -- Don't try to match the default style as a suffix
            suffix_to_remove = "{." .. class_name .. "}"
            -- Check if the original_text ends with the current suffix_to_remove
            if original_text:sub(-#suffix_to_remove) == suffix_to_remove then
              detected_class = class_name
              break -- Found a match, no need to check other classes
            end
          end
        end

        if detected_class then
          found_style_key = detected_class
          class_detected_and_processed = true

          -- Remove the detected `{.class}` suffix from the string
          -- original_text:sub(1, #original_text - #suffix_to_remove) takes the substring from char 1 up to the point just before the suffix
          last_inline.text = original_text:sub(1, #original_text - #suffix_to_remove)
        end
      end
    end
  end

  local current_style = box_styles[found_style_key]

  -- Construct tcolorbox options
  local current_tcb_options = {}
  for _, opt in ipairs(base_tcb_options) do
    table.insert(current_tcb_options, opt)
  end
  table.insert(current_tcb_options, "colback=" .. current_style.bg_color)
  table.insert(current_tcb_options, "colframe=" .. current_style.border_color)

  local begin_tcolorbox = "\\begin{tcolorbox}[" .. table.concat(current_tcb_options, ",") .. "]\n"
  local end_tcolorbox = "\\end{tcolorbox}\n"

  -- Build the list of blocks to replace the original blockquote
  local output_blocks = pandoc.List:new{}
  table.insert(output_blocks, pandoc.RawBlock('latex', begin_tcolorbox))

  -- Add the icon to the first actual content block
  if #el.content > 0 and current_style.icon_latex then
      local first_real_block = el.content[1]
      -- Ensure it's a paragraph before trying to insert into its inlines
      if first_real_block.tag == "Para" then
          -- Insert the icon at the beginning of the first paragraph's inlines
          table.insert(first_real_block.content, 1, pandoc.RawInline('latex', current_style.icon_latex))
      else
          -- If the first block isn't a paragraph, insert the icon as a new paragraph before the content
          table.insert(output_blocks, pandoc.Para({pandoc.RawInline('latex', current_style.icon_latex)}))
      end
  end

  -- Add the original blockquote content (now potentially modified with icon and the suffix removed)
  for _, block_item in ipairs(el.content) do
      table.insert(output_blocks, block_item)
  end

  table.insert(output_blocks, pandoc.RawBlock('latex', end_tcolorbox))

  return output_blocks
end

-- NEW: Filter for BulletLists
function BulletList(el)
  local apply_links_list_style = false
  local suffix_to_remove_from_str = nil
  local class_name_from_str = nil

  -- Step 1: Check if the {.links-list} is present as a string in the last item
  if #el.content > 0 then -- Check if there are list items
    local last_list_item = el.content[#el.content] -- Get the last list item

    -- A list item itself is a list of blocks. Usually, it's just one 'Plain' or 'Para' block.
    if #last_list_item > 0 then
      local last_block_in_item = last_list_item[#last_list_item]

      -- Check if the last block in the item is a Plain or Para and has inlines
      if (last_block_in_item.tag == "Plain" or last_block_in_item.tag == "Para") and #last_block_in_item.content > 0 then
        local last_inline_in_item = last_block_in_item.content[#last_block_in_item.content]

        if last_inline_in_item.tag == "Str" then
          local original_text = last_inline_in_item.text
          local expected_suffix = "{.links-list}"

          if original_text:sub(-#expected_suffix) == expected_suffix then
            apply_links_list_style = true
            class_name_from_str = "links-list"
            suffix_to_remove_from_str = expected_suffix
            -- Remove the suffix from the string
            last_inline_in_item.text = original_text:sub(1, #original_text - #suffix_to_remove_from_str)
          end
        end
      end
    end
  end

  -- Step 2: Apply styling if the "links-list" class was detected and processed
  if apply_links_list_style then
    local current_style = box_styles["links-list"]

    local current_tcb_options = {}
    local options_to_use = current_style.custom_tcb_options or base_tcb_options
    for _, opt in ipairs(options_to_use) do
      table.insert(current_tcb_options, opt)
    end

    table.insert(current_tcb_options, "colback=" .. current_style.bg_color)
    table.insert(current_tcb_options, "colframe=" .. current_style.border_color)

    local begin_tcolorbox = "\\begin{tcolorbox}[" .. table.concat(current_tcb_options, ",") .. "]\n"
    local end_tcolorbox = "\\end{tcolorbox}\n"

    -- Return a sequence of blocks: the tcolorbox begin, the original BulletList, and the tcolorbox end
    return {
      pandoc.RawBlock('latex', begin_tcolorbox),
      el, -- The original BulletList element itself (now with the suffix removed from its last item)
      pandoc.RawBlock('latex', end_tcolorbox)
    }
  end

  -- If no specific class "links-list" was found/processed, return nil to let Pandoc render the list normally.
  return nil
end