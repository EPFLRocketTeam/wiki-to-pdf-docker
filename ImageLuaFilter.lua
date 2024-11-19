function Image(elem)
  local src = elem.src
  local width, height

  -- Parse the "=widthxheight" syntax at the end of the source
  local src_clean, dimensions = src:match("^(.-)%s*=%s*(%d*)x(%d*)$")
  if dimensions then
    src = src_clean
    width = dimensions[1]
    height = dimensions[2]
  end

  -- Prepend the fixed base path
  src = "/app/ert_wiki" .. src

  -- Add width and height to attributes if they exist
  if width then
    elem.attributes["width"] = width .. "px"
  end
  if height then
    elem.attributes["height"] = height .. "px"
  end

  -- Preserve alignment or other classes (e.g., .align-center)
  if elem.classes then
    elem.attributes["class"] = table.concat(elem.classes, " ")
  end

  elem.src = src -- Update the source
  return elem
end

